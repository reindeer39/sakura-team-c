resource "sakuracr_packet_filter" "intern-team-c" {
    name        = "intern-team-c"
    description = "description"
}

resource "sakuracr_packet_filter_rules" "rules" {
    packet_filter_id = sakuracr_packet_filter.intern-team-c.id
    expression {
        protocol         = "tcp"
        destination_port = "22222"
    }

    expression {
        protocol         = "tcp"
        destination_port = "80"
    }

    expression {
        protocol         = "tcp"
        destination_port = "443"
    }

    expression {
        protocol    = "ip"
        allow       = false
        description = "Deny ALL"
    }
}
