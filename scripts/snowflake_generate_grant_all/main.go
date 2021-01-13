package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/chanzuckerberg/terraform-provider-snowflake/pkg/provider"
	"github.com/chanzuckerberg/terraform-provider-snowflake/pkg/resources"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/sirupsen/logrus"
	"gopkg.in/errgo.v2/fmt/errors"
)

const (
	topLvlComment string = `NOTE: Generated by scripts/snowflake_generate_grant_all.
	Changes made directly to this file will be overwritten.
	Make improvements there so everyone can benefit.
  The reason this module exists is that the provider only supports one grant resource per (database_name, schema_name, table_name, on_future, with_grant_option) tuple.
	For example, if you used this module to grant an ALL privilege to a role you couldn't grant a subset of the ALL privs to another role.`

	perPrivTypeVarName string = "per_privilege_grants"

	// TODO(el): grab this version directly from the provider
	snowflakeProviderVersion string = ">= 0.20.0"
)

type Variable struct {
	TType       string      `json:"type"`
	Description string      `json:"description"`
	Default     interface{} `json:"default"`
}

type ModuleTemplate struct {
	Comment   string                 `json:"//,omitempty"`
	Variables map[string]Variable    `json:"variable,omitempty"`
	Locals    map[string]interface{} `json:"locals,omitempty"`

	// resource type: resource name: arguments
	Resources map[string]map[string]map[string]interface{} `json:"resource,omitempty"`

	// required_providers: provider_name: version
	Terraform map[string]map[string]map[string]string `json:"terraform,omitempty"`
}

func main() {
	err := exec()
	if err != nil {
		logrus.Fatal(err)
	}
}

func exec() error {
	ciTests := []string{}

	grants := provider.GetGrantResources()
	for resourceName, grant := range grants {
		moduleName := moduleName(resourceName)
		ciTests = append(ciTests, moduleName)
		tf, err := generateModule(resourceName, grant)
		if err != nil {
			return err
		}
		testCode, err := generateTest(grant)
		if err != nil {
			return err
		}

		err = writeModule(moduleName, tf, []byte(testCode))
		if err != nil {
			return err
		}
	}
	return ensureCI(ciTests)
}

func moduleName(name string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s-all", name), "_", "-")
}

// Assume we're running from this directory
func writeModule(name string, tf []byte, testCode []byte) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// make sure dir is there
	moduleDir := path.Join(cwd, "..", "..", name)
	err = os.MkdirAll(moduleDir, 0755)
	if err != nil {
		return err
	}

	// write the file terraform file
	err = ioutil.WriteFile(path.Join(moduleDir, "main.tf.json"), tf, 0644)
	if err != nil {
		return err
	}

	// write the file terraform file
	return ioutil.WriteFile(path.Join(moduleDir, "module_test.go"), testCode, 0644)
}

func generateModule(name string, grant *resources.TerraformGrantResource) ([]byte, error) {
	logrus.Infof("Generating module for resource %s", name)
	privileges := grant.ValidPrivs.ToList()
	sort.Strings(privileges)

	m := &ModuleTemplate{
		Comment:   topLvlComment,
		Variables: map[string]Variable{},
		Locals: map[string]interface{}{
			"privileges": privileges,
		},
		Terraform: map[string]map[string]map[string]string{
			"required_providers": {
				"snowflake": map[string]string{
					"source":  "chanzuckerberg/snowflake",
					"version": snowflakeProviderVersion,
				},
			},
		},
	}

	// Grab the vars from the provider
	for elementName, config := range grant.Resource.Schema {
		// ignore these
		if elementName == "privilege" {
			continue
		}

		ttype, err := reverseType(config)
		if err != nil {
			return nil, err
		}

		m.Variables[elementName] = Variable{
			TType:       ttype,
			Description: config.Description,
			Default:     nil,
		}
	}

	// Add the extra per_privilege_grants variable
	perPrivTypeInner := []string{}
	defaultPrivTypes := []string{}
	if _, sharesOK := grant.Resource.Schema["shares"]; sharesOK {
		perPrivTypeInner = append(perPrivTypeInner, "shares = list(string)")
		defaultPrivTypes = append(defaultPrivTypes, "shares = []")
	}
	if _, rolesOK := grant.Resource.Schema["roles"]; rolesOK {
		perPrivTypeInner = append(perPrivTypeInner, "roles = list(string)")
		defaultPrivTypes = append(defaultPrivTypes, "roles = []")
	}

	defaultPrivType := fmt.Sprintf(
		"{ %s }",
		strings.Join(defaultPrivTypes, ", "),
	)

	m.Variables[perPrivTypeVarName] = Variable{
		TType: fmt.Sprintf("map(object({%s}))", strings.Join(perPrivTypeInner, ",")),
		Description: `A map of privileges to authorized roles and shares. Privileges must be UPPER case.
  This allows you to authorize extra roles/shares for specific privileges.`,
		Default: map[string]interface{}{},
	}

	// Generate the all grant resource
	resourceAll := map[string]interface{}{
		"for_each":  "${toset(local.privileges)}",
		"privilege": "${each.value}",
	}
	for elementName := range grant.Resource.Schema {
		switch elementName {
		case "privilege": // do nothing
		case "roles":
			resourceAll["roles"] = fmt.Sprintf(`${setunion(
				var.roles,
				lookup(var.per_privilege_grants, each.value, %s).roles,
				)}`, defaultPrivType)
		case "shares":
			resourceAll["shares"] = fmt.Sprintf(`${setunion(
				var.shares,
				lookup(var.per_privilege_grants, each.value, %s).shares,
				)}`, defaultPrivType)
		default:
			resourceAll[elementName] = fmt.Sprintf("${var.%s}", elementName)

		}
	}
	m.Resources = map[string]map[string]map[string]interface{}{
		name: {
			"all": resourceAll,
		},
	}

	// Done assembling, dump to json
	return json.MarshalIndent(m, "", "  ")
}

func reverseType(s *schema.Schema) (string, error) {
	switch t := s.Type; t {
	case schema.TypeBool:
		return "bool", nil
	case schema.TypeString:
		return "string", nil
	case schema.TypeSet:
		inner, err := reverseType(s.Elem.(*schema.Schema))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("set(%s)", inner), nil
	case schema.TypeList:
		resource := s.Elem.(*schema.Resource)
		innerElements := []string{}

		for name, s := range resource.Schema {
			inner, err := reverseType(s)
			if err != nil {
				return "", err
			}
			innerElements = append(innerElements, fmt.Sprintf("%s = %s", name, inner))
			sort.Strings(innerElements)
		}
		return fmt.Sprintf("list(object({ %s }))", strings.Join(innerElements, ", ")), nil
	default:
		return "", errors.Newf("Unrecognized type %s", t.String())
	}
}

func optString(s string) *string {
	return &s
}