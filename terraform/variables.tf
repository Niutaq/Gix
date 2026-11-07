variable "is_local" {
  description = "Flag to enable LocalStack configuration"
  type        = bool
  default     = false
}

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-central-1"
}

variable "environment" {
  description = "Deployment environment"
  type        = string
  default     = "dev"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "db_password" {
  description = "RDS administrator password. Provide via TF_VAR_db_password or a secret manager."
  type        = string
  sensitive   = true
}
