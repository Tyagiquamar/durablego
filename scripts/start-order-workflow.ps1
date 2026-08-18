param(
  [string]$Api = "http://localhost:8080",
  [string]$OrderId = "order-129331"
)

$body = @{
  namespace = "production"
  name = "order-processing"
  idempotency_key = $OrderId
  activities = @(
    @{ name = "validate"; task_queue = "default" },
    @{ name = "payment"; task_queue = "default"; depends_on = @("validate") },
    @{ name = "inventory"; task_queue = "default"; depends_on = @("validate") },
    @{ name = "email"; task_queue = "default"; depends_on = @("payment", "inventory") }
  )
} | ConvertTo-Json -Depth 8

Invoke-RestMethod -Method Post -Uri "$Api/v1/workflows" -ContentType "application/json" -Body $body

