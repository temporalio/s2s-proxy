data "aws_vpc" "main" {
  id = var.vpc_id
}

data "aws_subnets" "example" {
  filter {
    name   = "vpc-id"
    values = [var.vpc_id]
  }
}

locals {
  # Pick an existing subnet in the VPC.
  subnet_id = data.aws_subnets.example.ids[0]
}


resource "aws_security_group" "main" {
  name        = "${var.name_prefix}-secgroup"
  description = "Security group for ${var.name_prefix}"
  vpc_id      = data.aws_vpc.main.id

  tags = {
    Name = "${var.name_prefix}-secgroup"
  }
}

resource "aws_vpc_security_group_ingress_rule" "allow_ssh_ingress" {
  count = length(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.main.id
  cidr_ipv4         = var.allowed_ingress_cidrs[count.index]
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

resource "aws_vpc_security_group_ingress_rule" "allow_http_ingress" {
  count = length(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.main.id
  cidr_ipv4         = var.allowed_ingress_cidrs[count.index]
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
}

resource "aws_vpc_security_group_ingress_rule" "allow_temporal_server_ingress" {
  count = length(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.main.id
  cidr_ipv4         = var.allowed_ingress_cidrs[count.index]
  ip_protocol       = "tcp"
  from_port         = 7233
  to_port           = 7234
}

resource "aws_vpc_security_group_ingress_rule" "allow_s2s_proxy_ingress" {
  count = length(var.allowed_ingress_cidrs)

  security_group_id = aws_security_group.main.id
  cidr_ipv4         = var.allowed_ingress_cidrs[count.index]
  ip_protocol       = "tcp"
  from_port         = 5333
  to_port           = 5334
}

# Allow ingress from the same VPC cidr block.
resource "aws_vpc_security_group_ingress_rule" "allow_same_vpc_ingress" {
  count = length(data.aws_vpc.main.cidr_block_associations)

  security_group_id = aws_security_group.main.id
  cidr_ipv4         = data.aws_vpc.main.cidr_block_associations[count.index].cidr_block
  ip_protocol       = "-1"
}

resource "aws_vpc_security_group_egress_rule" "allow_all_egress_ipv4" {
  security_group_id = aws_security_group.main.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1" # semantically equivalent to all ports
}


data "aws_ami" "amazon_linux" {
  most_recent = true

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["137112412989"] # Amazon
}

resource "aws_key_pair" "main" {
  key_name   = "${var.name_prefix}-keypair"
  public_key = var.ssh_public_key
}

resource "aws_instance" "main" {
  ami                    = data.aws_ami.amazon_linux.id
  instance_type          = var.instance_type
  vpc_security_group_ids = [aws_security_group.main.id]
  key_name               = aws_key_pair.main.key_name
  subnet_id              = local.subnet_id

  tags = {
    Name = "${var.name_prefix}-temporal-server"
  }

  lifecycle {
    # don't recreate the instance just because a new ami was released.
    ignore_changes = [ami]
  }
}
