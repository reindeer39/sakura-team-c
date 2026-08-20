resource "sakura_monitoring_suite_log_storage" "app" {
  name                  = "intern2026-logs"
  retention_period_days = 30
}

resource "sakura_monitoring_suite_metric_storage" "app" {
  name = "intern2026-metrics"
}

resource "sakura_monitoring_suite_log_routing" "db_slow" {
  resource_id    = sakura_database.db.id
  storage_id     = sakura_monitoring_suite_log_storage.app.id
  publisher_code = "database"
  variant        = "slowquerylog"
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