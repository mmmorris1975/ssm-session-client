#!/usr/bin/env bash
# setup-github-oidc.sh
#
# One-time setup script: creates the GitHub Actions OIDC provider, IAM policy,
# and IAM role needed for the acceptance test workflow.
#
# These are long-lived resources — run once per AWS account, not on every
# tofu apply/destroy cycle.
#
# Usage:
#   AWS_REGION=ap-southeast-2 \
#   GITHUB_ORG=my-org \
#   ./test/scripts/setup-github-oidc.sh

set -euo pipefail

export AWS_PAGER=""  # Disable the AWS CLI pager (prevents vi/less opening for output).

GITHUB_ORG="${GITHUB_ORG:?Please set GITHUB_ORG}"
GITHUB_REPO="${GITHUB_REPO:-ssm-session-client}"
AWS_REGION="${AWS_REGION:-ap-southeast-2}"
ROLE_NAME="ssm-session-client-github-actions"
INLINE_POLICY_NAME="acceptance-test-permissions"

AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
OIDC_PROVIDER_URL="token.actions.githubusercontent.com"
OIDC_PROVIDER_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER_URL}"
STATE_BUCKET="ssm-session-client-tfstate-${AWS_ACCOUNT_ID}-${AWS_REGION}"

echo "==> AWS account   : ${AWS_ACCOUNT_ID}"
echo "==> Region        : ${AWS_REGION}"
echo "==> GitHub repo   : ${GITHUB_ORG}/${GITHUB_REPO}"
echo "==> Role name     : ${ROLE_NAME}"
echo "==> TF state bucket: ${STATE_BUCKET}"
echo ""

# ---------------------------------------------------------------------------
# Step 1: OIDC Identity Provider
# ---------------------------------------------------------------------------
echo "--- Step 1: GitHub OIDC Identity Provider"

if aws iam get-open-id-connect-provider \
      --open-id-connect-provider-arn "${OIDC_PROVIDER_ARN}" \
      --query OpenIDConnectProviderArn --output text 2>/dev/null; then
  echo "    Already exists: ${OIDC_PROVIDER_ARN}"
else
  aws iam create-open-id-connect-provider \
    --url "https://${OIDC_PROVIDER_URL}" \
    --client-id-list sts.amazonaws.com \
    --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1
  echo "    Created: ${OIDC_PROVIDER_ARN}"
fi

# ---------------------------------------------------------------------------
# Step 2: S3 bucket for Terraform state
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 2: S3 Terraform state bucket (${STATE_BUCKET})"

if aws s3api head-bucket --bucket "${STATE_BUCKET}" 2>/dev/null; then
  echo "    Already exists: s3://${STATE_BUCKET}"
else
  if [ "${AWS_REGION}" = "us-east-1" ]; then
    aws s3api create-bucket --bucket "${STATE_BUCKET}"
  else
    aws s3api create-bucket \
      --bucket "${STATE_BUCKET}" \
      --create-bucket-configuration LocationConstraint="${AWS_REGION}"
  fi
  echo "    Created: s3://${STATE_BUCKET}"
fi

aws s3api put-bucket-versioning \
  --bucket "${STATE_BUCKET}" \
  --versioning-configuration Status=Enabled
echo "    Versioning: enabled"

aws s3api put-bucket-encryption \
  --bucket "${STATE_BUCKET}" \
  --server-side-encryption-configuration '{
    "Rules": [{
      "ApplyServerSideEncryptionByDefault": {"SSEAlgorithm": "AES256"},
      "BucketKeyEnabled": true
    }]
  }'
echo "    Encryption: AES256"

aws s3api put-public-access-block \
  --bucket "${STATE_BUCKET}" \
  --public-access-block-configuration \
    "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"
echo "    Public access: blocked"

# ---------------------------------------------------------------------------
# Step 3: IAM Role
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 3: IAM role (${ROLE_NAME})"

TRUST_POLICY=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {
      "Federated": "${OIDC_PROVIDER_ARN}"
    },
    "Action": "sts:AssumeRoleWithWebIdentity",
    "Condition": {
      "StringLike": {
        "${OIDC_PROVIDER_URL}:sub": "repo:${GITHUB_ORG}/${GITHUB_REPO}:*"
      },
      "StringEquals": {
        "${OIDC_PROVIDER_URL}:aud": "sts.amazonaws.com"
      }
    }
  }]
}
EOF
)

if aws iam get-role --role-name "${ROLE_NAME}" --query Role.Arn --output text 2>/dev/null; then
  echo "    Already exists"
else
  aws iam create-role \
    --role-name "${ROLE_NAME}" \
    --description "Role assumed by GitHub Actions via OIDC for acceptance tests." \
    --assume-role-policy-document "${TRUST_POLICY}"
  echo "    Created"
fi

# ---------------------------------------------------------------------------
# Step 4: Inline policy (idempotent — put-role-policy overwrites on re-run)
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 4: Inline policy (${INLINE_POLICY_NAME})"

INLINE_POLICY_DOCUMENT=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SSMSession",
      "Effect": "Allow",
      "Action": [
        "ssm:StartSession",
        "ssm:TerminateSession",
        "ssm:DescribeSessions",
        "ssm:GetConnectionStatus",
        "ssm:DescribeInstanceInformation",
        "ssm:ListTagsForResource",
        "ssm:GetParameter"
      ],
      "Resource": "*"
    },
    {
      "Sid": "SSMMessages",
      "Effect": "Allow",
      "Action": [
        "ssmmessages:CreateControlChannel",
        "ssmmessages:CreateDataChannel",
        "ssmmessages:OpenControlChannel",
        "ssmmessages:OpenDataChannel"
      ],
      "Resource": "*"
    },
    {
      "Sid": "EC2TestRunner",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeInstanceStatus",
        "ec2:DescribeVpcs",
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeKeyPairs",
        "ec2:DescribeVpcEndpoints",
        "ec2:DescribeVpcEndpointServices",
        "ec2:DescribeNetworkInterfaces",
        "ec2:DescribeAvailabilityZones",
        "ec2:DescribeTags",
        "ec2:GetPasswordData",
        "ec2:DescribeVpcAttribute",
        "ec2:DescribeInstanceTypes"
        
      ],
      "Resource": "*"
    },
    {
      "Sid": "EC2TerraformManage",
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances",
        "ec2:TerminateInstances",
        "ec2:CreateSecurityGroup",
        "ec2:DeleteSecurityGroup",
        "ec2:AuthorizeSecurityGroupEgress",
        "ec2:RevokeSecurityGroupEgress",
        "ec2:AuthorizeSecurityGroupIngress",
        "ec2:RevokeSecurityGroupIngress",
        "ec2:ImportKeyPair",
        "ec2:DeleteKeyPair",
        "ec2:CreateVpcEndpoint",
        "ec2:DeleteVpcEndpoints",
        "ec2:ModifyVpcEndpoint",
        "ec2:ModifyInstanceAttribute",
        "ec2:CreateTags",
        "ec2:DeleteTags"
      ],
      "Resource": "*"
    },
    {
      "Sid": "EC2InstanceConnect",
      "Effect": "Allow",
      "Action": ["ec2-instance-connect:SendSSHPublicKey"],
      "Resource": "arn:aws:ec2:${AWS_REGION}:${AWS_ACCOUNT_ID}:instance/*"
    },
    {
      "Sid": "IAMTerraformManage",
      "Effect": "Allow",
      "Action": [
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:GetRole",
        "iam:TagRole",
        "iam:UntagRole",
        "iam:UpdateAssumeRolePolicy",
        "iam:PutRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:GetRolePolicy",
        "iam:ListRolePolicies",
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy",
        "iam:ListAttachedRolePolicies",
        "iam:CreatePolicy",
        "iam:DeletePolicy",
        "iam:GetPolicy",
        "iam:TagPolicy",
        "iam:GetPolicyVersion",
        "iam:CreatePolicyVersion",
        "iam:DeletePolicyVersion",
        "iam:ListPolicyVersions",
        "iam:CreateInstanceProfile",
        "iam:DeleteInstanceProfile",
        "iam:GetInstanceProfile",
        "iam:AddRoleToInstanceProfile",
        "iam:RemoveRoleFromInstanceProfile",
        "iam:PassRole"
      ],
      "Resource": "*"
    },
    {
      "Sid": "KMSTerraformManage",
      "Effect": "Allow",
      "Action": [
        "kms:CreateKey",
        "kms:DescribeKey",
        "kms:GetKeyPolicy",
        "kms:PutKeyPolicy",
        "kms:ScheduleKeyDeletion",
        "kms:EnableKeyRotation",
        "kms:DisableKeyRotation",
        "kms:GetKeyRotationStatus",
        "kms:CreateAlias",
        "kms:DeleteAlias",
        "kms:ListAliases",
        "kms:TagResource",
        "kms:UntagResource",
        "kms:GenerateDataKey",
        "kms:Decrypt",
        "kms:ListResourceTags"

      ],
      "Resource": "*"
    },
    {
      "Sid": "Route53TerraformManage",
      "Effect": "Allow",
      "Action": [
        "route53:CreateHostedZone",
        "route53:DeleteHostedZone",
        "route53:GetHostedZone",
        "route53:ListHostedZones",
        "route53:ListHostedZonesByName",
        "route53:ChangeResourceRecordSets",
        "route53:ListResourceRecordSets",
        "route53:AssociateVPCWithHostedZone",
        "route53:DisassociateVPCFromHostedZone",
        "route53:GetChange"
      ],
      "Resource": "*"
    },
    {
      "Sid": "TerraformStateS3",
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket",
        "s3:GetBucketVersioning",
        "s3:GetEncryptionConfiguration"
      ],
      "Resource": [
        "arn:aws:s3:::${STATE_BUCKET}",
        "arn:aws:s3:::${STATE_BUCKET}/*"
      ]
    }
  ]
}
EOF
)

aws iam put-role-policy \
  --role-name "${ROLE_NAME}" \
  --policy-name "${INLINE_POLICY_NAME}" \
  --policy-document "${INLINE_POLICY_DOCUMENT}"
echo "    Applied inline policy to ${ROLE_NAME}"

# ---------------------------------------------------------------------------
# Step 5: Set GitHub Actions secret
# ---------------------------------------------------------------------------
echo ""
echo "--- Step 5: GitHub Actions secret (AWS_ACCEPTANCE_ROLE_ARN)"

ROLE_ARN=$(aws iam get-role --role-name "${ROLE_NAME}" --query Role.Arn --output text)

if command -v gh &>/dev/null && gh auth status &>/dev/null; then
  gh secret set AWS_ACCEPTANCE_ROLE_ARN \
    --repo "${GITHUB_ORG}/${GITHUB_REPO}" \
    --body "${ROLE_ARN}"
  echo "    Secret set on ${GITHUB_ORG}/${GITHUB_REPO}"
else
  echo "    gh CLI not available or not authenticated — set the secret manually:"
  echo ""
  echo "    Settings -> Secrets and variables -> Actions -> New repository secret"
  echo "      Name : AWS_ACCEPTANCE_ROLE_ARN"
  echo "      Value: ${ROLE_ARN}"
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
echo ""
echo "==> Setup complete."
echo "    Role ARN     : ${ROLE_ARN}"
echo "    State bucket : s3://${STATE_BUCKET}"
