const fs = require('fs');
const events = JSON.parse(fs.readFileSync('events.json', 'utf8'));
const traceId = 'e69cfa55fa116e2def8bdfd2a4036d17';
const traceEvents = events.filter(e => e.trace_id === traceId);
fs.writeFileSync('trace_example.json', JSON.stringify(traceEvents, null, 2));

const metricEvent = events.find(e => e.event_type === 'metric');
fs.writeFileSync('metric_example.json', JSON.stringify(metricEvent, null, 2));

console.log('done');
