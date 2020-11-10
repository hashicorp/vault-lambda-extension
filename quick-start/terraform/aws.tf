//--------------------------------------------------------------------
// Providers

provider "aws" {
  // Credentials set via env vars
  version = "~> 3.6.0"
  region  = var.aws_region
}

//--------------------------------------------------------------------
// Data Sources

data "aws_ami" "ubuntu" {
  most_recent = true

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-bionic-18.04-amd64-server-*"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }

  owners = ["099720109477"] # Canonical
}

resource "aws_internet_gateway" "gate" {
  vpc_id = aws_vpc.vpc.id

  tags = {
    Name = "vault-lamba-vpc-gate"
  }
}

resource "aws_vpc" "vpc" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true

  tags = {
    Name = "vault-lamba-db-vpc"
  }
}

resource "aws_subnet" "first_subnet" {
  vpc_id                  = aws_vpc.vpc.id
  cidr_block              = var.subnet_cidr_one
  availability_zone       = var.aws_zone
  map_public_ip_on_launch = true

  tags = {
    Name = "vault-lamba-db-subnet"
  }
}

resource "aws_subnet" "second_subnet" {
  vpc_id                  = aws_vpc.vpc.id
  cidr_block              = var.subnet_cidr_two
  availability_zone       = var.aws_zone_alternative
  map_public_ip_on_launch = true

  tags = {
    Name = "vault-lamba-db-subnet-alternative"
  }
}


resource "aws_db_subnet_group" "main_db" {
  name       = "main"
  subnet_ids = [ aws_subnet.first_subnet.id , aws_subnet.second_subnet.id ]

  tags = {
    Name = "My DB subnet"
  }
}
