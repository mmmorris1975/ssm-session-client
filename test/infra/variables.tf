variable "region" {
  description = "AWS region for test infrastructure."
  type        = string
  default     = "ap-southeast-2"
}

variable "instance_type" {
  description = "EC2 instance type for test instances."
  type        = string
  default     = "t3.micro"
}

variable "environment" {
  description = "Value for the Environment tag on all resources."
  type        = string
  default     = "acceptance-test"
}

variable "name_prefix" {
  description = "Name prefix used for all resource names and tags."
  type        = string
  default     = "ssm-session-client-test"
}

variable "create_dns_record" {
  description = "Create a Route53 private hosted zone with a TXT record for DNS-resolver tests."
  type        = bool
  default     = false
}

variable "create_kms_key" {
  description = "Create a KMS key for encryption-channel tests."
  type        = bool
  default     = true
}

variable "create_vpc_endpoints" {
  description = "Create VPC Interface endpoints for SSM/EC2/KMS to test --*-endpoint flags."
  type        = bool
  default     = false
}

variable "create_windows_instance" {
  description = "Create an optional Windows Server 2022 EC2 instance for RDP tests."
  type        = bool
  default     = true
}

variable "windows_instance_type" {
  description = "EC2 instance type for the Windows test instance (Windows needs more RAM than Linux)."
  type        = string
  default     = "t3.medium"
}
