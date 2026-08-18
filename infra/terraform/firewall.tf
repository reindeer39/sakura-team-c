resource "sakura_packet_filter" "intern-team-c" {
  name        = "intern-team-c"
  description = "description"
}

resource "sakura_packet_filter_rules" "intern-team-c" {
  packet_filter_id = sakura_packet_filter.intern-team-c.id
  expression = [{
    protocol         = "tcp"
    destination_port = "22222"
    },
    {
      protocol         = "tcp"
      destination_port = "80"
    },
    {
      protocol         = "tcp"
      destination_port = "443"
    },
    {
      protocol    = "ip"
      allow       = false
      description = "Deny ALL"
  }]
}
