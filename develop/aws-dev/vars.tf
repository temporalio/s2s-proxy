variable "name_prefix" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "allowed_ingress_cidrs" {
  type = list(string)
}

variable "ssh_public_key" {
  type = string
}

variable "instance_type" {
  type    = string
  default = "t3a.medium"
}
