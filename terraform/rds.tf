resource "aws_db_subnet_group" "db_subnets" {
  name       = "gix-db-subnet-group"
  subnet_ids = aws_subnet.private[*].id

  tags = {
    Name = "Gix DB Subnet Group"
  }
}

resource "aws_security_group" "db_sg" {
  name        = "gix-db-sg"
  description = "Allow inbound traffic from EKS"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = [aws_vpc.main.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_instance" "timescale" {
  identifier           = "gix-db-${var.environment}"
  allocated_storage    = 20
  storage_type         = "gp3"
  engine               = "postgres"
  engine_version       = "16.1"
  instance_class       = "db.t4g.micro" # FinOps: Graviton is cheaper
  db_name              = "gix"
  username             = "gix_admin"
  password             = "ChangeMe123!"
  db_subnet_group_name = aws_db_subnet_group.db_subnets.name
  vpc_security_group_ids = [aws_security_group.db_sg.id]

  skip_final_snapshot  = true
  publicly_accessible  = false

  tags = {
    "focus:service_category" = "database"
  }
}
