package test

import (
	"fmt"
	"testing"

	"github.com/chanzuckerberg/go-misc/tftest"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
)

func TestAWSIAMRoleInfraCI(t *testing.T) {
	test := tftest.Test{
		Setup: func(t *testing.T) *terraform.Options {
			curAcct := tftest.AWSCurrentAccountID(t)

			project := tftest.UniqueID()
			env := tftest.UniqueID()
			service := tftest.UniqueID()
			owner := tftest.UniqueID()

			return tftest.Options(
				tftest.IAMRegion,
				map[string]interface{}{
					"role_name":         random.UniqueId(),
					"source_account_id": curAcct,
					"project":           project,
					"env":               env,
					"service":           service,
					"owner":             owner,
					"iam_path":          fmt.Sprintf("/%s/", random.UniqueId()),
				},
			)
		},
		Validate: func(t *testing.T, options *terraform.Options) {},
	}

	test.Run(t)
}
