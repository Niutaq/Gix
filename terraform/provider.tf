provider "aws" {
  region = var.aws_region

  # These parameters are required for LocalStack, but do not interfere with AWS
  access_key                  = var.is_local ? "test" : null
  secret_key                  = var.is_local ? "test" : null
  skip_credentials_validation = var.is_local
  skip_metadata_api_check     = var.is_local
  skip_requesting_account_id  = var.is_local

  dynamic "endpoints" {
    for_each = var.is_local ? [1] : []
    content {
      ec2        = "http://localhost:4566"
      rds        = "http://localhost:4566"
      s3         = "http://localhost:4566"
      iam        = "http://localhost:4566"
      cloudwatch = "http://localhost:4566"
      eks        = "http://localhost:4566"
      sts        = "http://localhost:4566"
    }
  }

  default_tags {
    tags = {
      Project     = "Gix"
      Environment = var.environment
      ManagedBy   = "Terraform"
      # FinOps FOCUS standard tags mapping
      "focus:project"     = "gix-currency-app"
      "focus:environment" = var.environment
      "focus:owner"       = "niutaq"
    }
  }
}

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}
