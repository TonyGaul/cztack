variable project {
  type        = string
  description = "Project for tagging and naming. See [doc](../README.md#consistent-tagging)"
}

variable env {
  type        = string
  description = "Env for tagging and naming. See [doc](../README.md#consistent-tagging)"
}

variable service {
  type        = string
  description = "Service for tagging and naming. See [doc](../README.md#consistent-tagging)"
}

variable owner {
  type        = string
  description = "Owner for tagging and naming. See [doc](../README.md#consistent-tagging)"
}

variable iam_path {
  type        = string
  description = "The IAM path under which the IAM role will be created."
  default     = null
}

variable principals {
  type        = list(object({ type = string, identifiers = list(string) }))
  description = "AWS IAM Principals which will be able to assume this role."
}

variable role_name {
  type        = string
  description = "IAM role name."
}

variable role_description {
  type        = string
  description = "IAM role description."
  default     = null
}

variable inline_policies {
  type        = list(object({ name = string, policy = string }))
  description = "List of inline policies to be associated with the IAM role."
  default     = []
}

variable attached_policies_arns {
  type        = list(string)
  description = "List of policies ARNs to be attached to the IAM role."
  default     = []
}
