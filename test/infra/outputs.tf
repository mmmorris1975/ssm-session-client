output "instance_id" {
  description = "Primary test EC2 instance ID."
  value       = aws_instance.test.id
}

output "instance_private_ip" {
  description = "Private IPv4 address of the primary test instance."
  value       = aws_instance.test.private_ip
}

output "instance_tag_name" {
  description = "Value of the Name tag on the primary test instance."
  value       = var.name_prefix
}

output "aws_region" {
  description = "AWS region where test infrastructure is deployed."
  value       = var.region
}

output "test_alias_tag_key" {
  description = "Tag key used for alias-resolver tests."
  value       = "Name"
}

output "test_alias_tag_value" {
  description = "Tag value used for alias-resolver tests."
  value       = var.name_prefix
}

output "dns_hostname" {
  description = "Route53 DNS hostname pointing to the test instance (empty if create_dns_record=false)."
  value       = var.create_dns_record ? aws_route53_record.test_txt[0].fqdn : ""
}

output "kms_key_arn" {
  description = "ARN of the KMS key for encryption tests (empty if create_kms_key=false)."
  value       = var.create_kms_key ? aws_kms_key.test[0].arn : ""
}

output "windows_instance_id" {
  description = "Windows Server test instance ID (empty if create_windows_instance=false)."
  value       = var.create_windows_instance ? aws_instance.windows_test[0].id : ""
}

output "rdp_key_pair_file" {
  description = "Local path to the PEM private key for RDP --get-password tests (empty if create_windows_instance=false)."
  value       = var.create_windows_instance ? local_sensitive_file.rdp_private_key[0].filename : ""
  sensitive   = true
}

# Write a flat JSON file consumed by the Go acceptance tests.
resource "local_file" "outputs_json" {
  filename        = "${path.module}/outputs.json"
  file_permission = "0600"
  content = jsonencode({
    instance_id          = aws_instance.test.id
    instance_private_ip  = aws_instance.test.private_ip
    instance_tag_name    = var.name_prefix
    aws_region           = var.region
    test_alias_tag_key   = "Name"
    test_alias_tag_value = var.name_prefix
    dns_hostname         = var.create_dns_record ? aws_route53_record.test_txt[0].fqdn : ""
    kms_key_arn          = var.create_kms_key ? aws_kms_key.test[0].arn : ""
    windows_instance_id  = var.create_windows_instance ? aws_instance.windows_test[0].id : ""
    rdp_key_pair_file    = var.create_windows_instance ? local_sensitive_file.rdp_private_key[0].filename : ""
  })
}
