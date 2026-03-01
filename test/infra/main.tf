terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }

  # Backend config is supplied at init time via -backend-config flags.
  # Run test/scripts/setup-github-oidc.sh once to create the bucket, then
  # use the Makefile (acceptance-prepare / acceptance-destroy) which pass
  # the correct -backend-config arguments automatically.
  backend "s3" {}
}

provider "aws" {
  region = var.region
}

# ---------------------------------------------------------------------------
# Data sources
# ---------------------------------------------------------------------------

data "aws_caller_identity" "current" {}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
  filter {
    name   = "default-for-az"
    values = ["true"]
  }
}

# Latest Amazon Linux 2023 x86_64 AMI via SSM Parameter Store.
data "aws_ssm_parameter" "al2023_ami" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}

# ---------------------------------------------------------------------------
# Security group — no inbound SSH; SSM agent reaches AWS endpoints via egress
# ---------------------------------------------------------------------------

resource "aws_security_group" "test_instance" {
  name        = "${var.name_prefix}-sg"
  description = "ssm-session-client acceptance test instances: SSM-only, no direct SSH."
  vpc_id      = data.aws_vpc.default.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic."
  }

  tags = {
    Name        = "${var.name_prefix}-sg"
    Environment = var.environment
  }
}

# ---------------------------------------------------------------------------
# Primary test EC2 instance
# ---------------------------------------------------------------------------

resource "aws_instance" "test" {
  ami                    = data.aws_ssm_parameter.al2023_ami.value
  instance_type          = var.instance_type
  subnet_id              = tolist(data.aws_subnets.default.ids)[0]
  iam_instance_profile   = aws_iam_instance_profile.test_instance.name
  vpc_security_group_ids = [aws_security_group.test_instance.id]

  # IMDSv2 required.
  metadata_options {
    http_tokens                 = "required"
    http_endpoint               = "enabled"
    http_put_response_hop_limit = 2
  }

  tags = {
    Name        = var.name_prefix
    Environment = var.environment
    TestRole    = "primary"
  }
}

# ---------------------------------------------------------------------------
# Optional: Route53 private hosted zone + TXT record for DNS-resolver tests
# ---------------------------------------------------------------------------

resource "aws_route53_zone" "test" {
  count = var.create_dns_record ? 1 : 0
  name  = "${var.name_prefix}.internal"

  vpc {
    vpc_id = data.aws_vpc.default.id
  }

  tags = {
    Name        = "${var.name_prefix}-zone"
    Environment = var.environment
  }
}

resource "aws_route53_record" "test_txt" {
  count   = var.create_dns_record ? 1 : 0
  zone_id = aws_route53_zone.test[0].zone_id
  name    = "instance.${var.name_prefix}.internal"
  type    = "TXT"
  ttl     = 60
  records = [aws_instance.test.id]
}

# ---------------------------------------------------------------------------
# Optional: KMS key for encryption-channel tests
# ---------------------------------------------------------------------------

resource "aws_kms_key" "test" {
  count               = var.create_kms_key ? 1 : 0
  description         = "ssm-session-client acceptance test encryption key."
  enable_key_rotation = true

  tags = {
    Name        = "${var.name_prefix}-kms"
    Environment = var.environment
  }
}

resource "aws_kms_alias" "test" {
  count         = var.create_kms_key ? 1 : 0
  name          = "alias/${var.name_prefix}"
  target_key_id = aws_kms_key.test[0].key_id
}

# ---------------------------------------------------------------------------
# Optional: VPC Interface endpoints for --*-endpoint flag tests
# ---------------------------------------------------------------------------

data "aws_vpc_endpoint_service" "ssm" {
  count        = var.create_vpc_endpoints ? 1 : 0
  service      = "ssm"
  service_type = "Interface"
}

data "aws_vpc_endpoint_service" "ssmmessages" {
  count        = var.create_vpc_endpoints ? 1 : 0
  service      = "ssmmessages"
  service_type = "Interface"
}

resource "aws_vpc_endpoint" "ssm" {
  count               = var.create_vpc_endpoints ? 1 : 0
  vpc_id              = data.aws_vpc.default.id
  service_name        = data.aws_vpc_endpoint_service.ssm[0].service_name
  vpc_endpoint_type   = "Interface"
  subnet_ids          = tolist(data.aws_subnets.default.ids)
  security_group_ids  = [aws_security_group.test_instance.id]
  private_dns_enabled = true

  tags = {
    Name        = "${var.name_prefix}-ssm-endpoint"
    Environment = var.environment
  }
}

resource "aws_vpc_endpoint" "ssmmessages" {
  count               = var.create_vpc_endpoints ? 1 : 0
  vpc_id              = data.aws_vpc.default.id
  service_name        = data.aws_vpc_endpoint_service.ssmmessages[0].service_name
  vpc_endpoint_type   = "Interface"
  subnet_ids          = tolist(data.aws_subnets.default.ids)
  security_group_ids  = [aws_security_group.test_instance.id]
  private_dns_enabled = true

  tags = {
    Name        = "${var.name_prefix}-ssmmessages-endpoint"
    Environment = var.environment
  }
}

# ---------------------------------------------------------------------------
# Optional: Windows Server 2022 instance for RDP tests
# ---------------------------------------------------------------------------

# Latest Windows Server 2022 English Full Base AMI.
data "aws_ssm_parameter" "windows_ami" {
  count = var.create_windows_instance ? 1 : 0
  name  = "/aws/service/ami-windows-latest/Windows_Server-2022-English-Full-Base"
}

# RSA key pair for --get-password tests.  The private key is written to
# test/infra/rdp_test_key.pem and referenced in outputs.json.
# Keep this file out of version control (.gitignore covers *.pem).
resource "tls_private_key" "rdp_test" {
  count     = var.create_windows_instance ? 1 : 0
  algorithm = "RSA"
  rsa_bits  = 4096
}

resource "aws_key_pair" "rdp_test" {
  count      = var.create_windows_instance ? 1 : 0
  key_name   = "${var.name_prefix}-rdp-test"
  public_key = tls_private_key.rdp_test[0].public_key_openssh

  tags = {
    Name        = "${var.name_prefix}-rdp-key"
    Environment = var.environment
  }
}

resource "local_sensitive_file" "rdp_private_key" {
  count           = var.create_windows_instance ? 1 : 0
  filename        = "${path.module}/rdp_test_key.pem"
  content         = tls_private_key.rdp_test[0].private_key_pem
  file_permission = "0600"
}

resource "aws_instance" "windows_test" {
  count                  = var.create_windows_instance ? 1 : 0
  ami                    = data.aws_ssm_parameter.windows_ami[0].value
  instance_type          = var.windows_instance_type
  subnet_id              = tolist(data.aws_subnets.default.ids)[0]
  iam_instance_profile   = aws_iam_instance_profile.test_instance.name
  vpc_security_group_ids = [aws_security_group.test_instance.id]
  key_name               = aws_key_pair.rdp_test[0].key_name

  # IMDSv2 required.
  metadata_options {
    http_tokens                 = "required"
    http_endpoint               = "enabled"
    http_put_response_hop_limit = 2
  }

  tags = {
    Name        = "${var.name_prefix}-windows"
    Environment = var.environment
    TestRole    = "windows-rdp"
    OS          = "Windows"
  }
}
