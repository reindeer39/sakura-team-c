resource "sakura_monitoring_suite_log_storage" "system" {
  name = "intern2026-logs-system"
}

resource "sakura_monitoring_suite_log_storage" "app" {
  name                  = "intern2026-logs"
  retention_period_days = 40
}

resource "sakura_monitoring_suite_metric_storage" "system" {
  name = "intern2026-metrics-system"
}

resource "sakura_monitoring_suite_metric_storage" "app" {
  name = "intern2026-metrics"
}

resource "sakura_monitoring_suite_log_routing" "db_system" {
  resource_id    = sakura_database.db.id
  storage_id     = sakura_monitoring_suite_log_storage.app.id
  publisher_code = "database"
  variant        = "systemlog"
}

resource "sakura_monitoring_suite_metric_routing" "db" {
  resource_id    = sakura_database.db.id
  storage_id     = sakura_monitoring_suite_metric_storage.app.id
  publisher_code = "database"
  variant        = "systemmetrics"
}

resource "sakura_monitoring_suite_log_storage_access_key" "agent" {
  storage_id = sakura_monitoring_suite_log_storage.app.id
}

resource "sakura_monitoring_suite_metric_storage_access_key" "agent" {
  storage_id = sakura_monitoring_suite_metric_storage.app.id
}

output "monitoring_logs_endpoint" {
  value = sakura_monitoring_suite_log_storage.app.id
}

output "monitoring_logs_token" {
  value     = sakura_monitoring_suite_log_storage_access_key.agent.secret
  sensitive = true
}

output "monitoring_metrics_endpoint" {
  value = sakura_monitoring_suite_metric_storage.app.id
}

output "monitoring_metrics_token" {
  value     = sakura_monitoring_suite_metric_storage_access_key.agent.secret
  sensitive = true
}
