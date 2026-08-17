$events = Invoke-RestMethod -Uri "http://localhost:8081/api/v1/events"
$trace = $events | Where-Object { $_.trace_id -eq 'e69cfa55fa116e2def8bdfd2a4036d17' }
$trace | ConvertTo-Json -Depth 5 | Out-File -FilePath trace_example.json -Encoding UTF8
$metric = $events | Where-Object { $_.event_type -eq 'metric' } | Select-Object -First 1
$metric | ConvertTo-Json -Depth 5 | Out-File -FilePath metric_example.json -Encoding UTF8
