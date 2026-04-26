module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 19.0"

  cluster_name    = "gix-cluster-${var.environment}"
  cluster_version = "1.30"

  cluster_endpoint_public_access = true

  vpc_id                   = aws_vpc.main.id
  subnet_ids               = aws_subnet.private[*].id
  control_plane_subnet_ids = aws_subnet.public[*].id

  # FinOps: Managed Node Groups + SPOT instances
  eks_managed_node_groups = {
    spot_nodes = {
      min_size     = 1
      max_size     = 3
      desired_size = 1

      instance_types = ["t3.medium", "t3.large"]
      capacity_type  = "SPOT"

      labels = {
        Environment = var.environment
        Project     = "Gix"
      }
    }
  }

  tags = {
    "focus:service_category" = "compute"
  }
}
