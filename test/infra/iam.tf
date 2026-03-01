# ---------------------------------------------------------------------------
# IAM role for test EC2 instances
# ---------------------------------------------------------------------------

resource "aws_iam_role" "test_instance" {
  name        = "${var.name_prefix}-instance-role"
  description = "Role assumed by ssm-session-client acceptance test EC2 instances."

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Action    = "sts:AssumeRole"
      Principal = { Service = "ec2.amazonaws.com" }
    }]
  })

  tags = {
    Name        = "${var.name_prefix}-instance-role"
    Environment = var.environment
  }
}

resource "aws_iam_role_policy_attachment" "ssm_managed_core" {
  role       = aws_iam_role.test_instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# Allow the instance to receive EC2 Instance Connect public keys.
resource "aws_iam_role_policy" "instance_connect" {
  name = "${var.name_prefix}-instance-connect"
  role = aws_iam_role.test_instance.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = "ec2-instance-connect:SendSSHPublicKey"
      Resource = "arn:aws:ec2:${var.region}:${data.aws_caller_identity.current.account_id}:instance/*"
      Condition = {
        StringEquals = { "ec2:osuser" = "ec2-user" }
      }
    }]
  })
}

resource "aws_iam_instance_profile" "test_instance" {
  name = "${var.name_prefix}-instance-profile"
  role = aws_iam_role.test_instance.name
}


