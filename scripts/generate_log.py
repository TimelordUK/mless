#!/usr/bin/env python3
"""Generate realistic Serilog-style log files for testing mless."""

import argparse
import random
from datetime import datetime, timedelta

LEVELS = ['TRC', 'DBG', 'INF', 'WRN', 'ERR', 'FTL']
# Realistic production distribution: mostly INFO with DEBUG, rare errors
LEVEL_WEIGHTS = [2, 25, 60, 8, 4, 1]  # TRC 2%, DBG 25%, INF 60%, WRN 8%, ERR 4%, FTL 1%

COMPONENTS = [
    'HttpServer', 'Database', 'Cache', 'Auth', 'Scheduler',
    'MessageQueue', 'FileProcessor', 'ApiGateway', 'Metrics', 'Config'
]

MESSAGES = {
    'TRC': [
        'Entering method {Method} with parameters {Params}',
        'Variable state: {State}',
        'Loop iteration {Index} of {Total}',
    ],
    'DBG': [
        'Processing request {RequestId}',
        'Cache lookup for key {Key}',
        'Parsed configuration: {Config}',
        'Connection pool status: {Active}/{Max} active',
        'Query execution plan generated',
    ],
    'INF': [
        'GET /api/v1/users/{UserId} completed in {Duration}ms',
        'POST /api/v1/orders completed in {Duration}ms',
        'GET /api/v1/products?page={Index} completed in {Duration}ms',
        'PUT /api/v1/users/{UserId}/profile completed in {Duration}ms',
        'DELETE /api/v1/sessions/{RequestId} completed in {Duration}ms',
        'Request {RequestId} completed in {Duration}ms',
        'User {UserId} authenticated successfully',
        'Scheduled job {JobName} started',
        'Scheduled job {JobName} completed in {Duration}ms',
        'File {FileName} processed, {Records} records',
        'Service started on port {Port}',
        'Health check passed',
        'Configuration reloaded',
        'Connected to database {Database}',
        'Message published to {Queue}',
        'Message consumed from {Queue} in {Duration}ms',
        'Batch processing completed: {Processed}/{Total} items',
        'Cache hit for key {Key}',
        'Database query executed in {Duration}ms',
        'Outbound HTTP call to {Service} completed in {Duration}ms',
    ],
    'WRN': [
        'Slow query detected: {Duration}ms for {Query}',
        'Rate limit approaching for client {ClientId}',
        'Retry attempt {Attempt} for operation {Operation}',
        'Memory usage at {Percent}%',
        'Certificate expires in {Days} days',
        'Deprecated API endpoint called: {Endpoint}',
        'Connection pool exhausted, waiting for available connection',
    ],
    'ERR': [
        'Failed to process request {RequestId}: {Error}',
        'Database connection failed: {Error}',
        'Authentication failed for user {UserId}',
        'Timeout waiting for response from {Service}',
        'Invalid message format: {Details}',
        'File not found: {FileName}',
        'Unhandled exception in {Component}: {Error}',
    ],
    'FTL': [
        'Application startup failed: {Error}',
        'Critical resource unavailable: {Resource}',
        'Data corruption detected in {Table}',
        'Unrecoverable error, shutting down',
    ],
}

def random_id():
    return ''.join(random.choices('0123456789abcdef', k=8))

def random_duration():
    return random.choice([
        random.randint(1, 50),
        random.randint(50, 200),
        random.randint(200, 1000),
        random.randint(1000, 5000),
    ])

def fill_template(template):
    """Replace placeholders with realistic values."""
    replacements = {
        '{RequestId}': random_id(),
        '{UserId}': f'user_{random.randint(1000, 9999)}',
        '{Duration}': str(random_duration()),
        '{Port}': str(random.choice([8080, 443, 3000, 5000, 9000])),
        '{Key}': f'cache:{random.choice(["user", "session", "config"])}:{random_id()}',
        '{Config}': '{"timeout": 30, "retries": 3}',
        '{Active}': str(random.randint(1, 20)),
        '{Max}': '20',
        '{JobName}': random.choice(['CleanupJob', 'ReportGenerator', 'DataSync', 'BackupJob']),
        '{FileName}': f'{random.choice(["data", "export", "import", "report"])}_{random_id()}.csv',
        '{Records}': str(random.randint(100, 10000)),
        '{Database}': random.choice(['postgres://db:5432/app', 'mysql://db:3306/app']),
        '{Queue}': random.choice(['orders', 'notifications', 'events', 'tasks']),
        '{Processed}': str(random.randint(100, 1000)),
        '{Total}': str(random.randint(1000, 1100)),
        '{Query}': 'SELECT * FROM users WHERE...',
        '{ClientId}': f'client_{random_id()}',
        '{Attempt}': str(random.randint(1, 5)),
        '{Operation}': random.choice(['SendEmail', 'ProcessPayment', 'SyncData']),
        '{Percent}': str(random.randint(70, 95)),
        '{Days}': str(random.randint(1, 30)),
        '{Endpoint}': f'/api/v1/{random.choice(["users", "orders", "legacy"])}',
        '{Error}': random.choice([
            'Connection refused',
            'Timeout exceeded',
            'Invalid token',
            'Resource not found',
            'Permission denied',
        ]),
        '{Service}': random.choice(['UserService', 'PaymentService', 'NotificationService']),
        '{Details}': 'Unexpected field "foo" at position 42',
        '{Component}': random.choice(COMPONENTS),
        '{Resource}': random.choice(['Database', 'Redis', 'MessageBroker']),
        '{Table}': random.choice(['users', 'orders', 'transactions']),
        '{Method}': random.choice(['ProcessOrder', 'ValidateInput', 'Transform']),
        '{Params}': '{id: 123, type: "full"}',
        '{State}': '{counter: 42, flag: true}',
        '{Index}': str(random.randint(1, 100)),
    }

    result = template
    for key, value in replacements.items():
        result = result.replace(key, value)
    return result

def generate_log_line(timestamp, format_style):
    """Generate a single log line."""
    level = random.choices(LEVELS, weights=LEVEL_WEIGHTS)[0]
    component = random.choice(COMPONENTS)
    message = fill_template(random.choice(MESSAGES[level]))

    if format_style == 'bracket':
        # [2024-01-15 10:30:45.123 INF] [Component] Message
        return f'[{timestamp.strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]} {level}] [{component}] {message}'
    else:
        # 2024-01-15 10:30:45.123 [INF] Component: Message
        return f'{timestamp.strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]} [{level}] {component}: {message}'

def main():
    parser = argparse.ArgumentParser(description='Generate test log files')
    parser.add_argument('lines', type=int, help='Number of lines to generate')
    parser.add_argument('-o', '--output', default='test.log', help='Output file')
    parser.add_argument('-f', '--format', choices=['bracket', 'standard'],
                        default='standard', help='Log format style')
    parser.add_argument('-s', '--seed', type=int, help='Random seed for reproducibility')

    args = parser.parse_args()

    if args.seed:
        random.seed(args.seed)

    # Start time, advance 10-5000ms per line
    timestamp = datetime.now() - timedelta(hours=random.randint(1, 24))

    with open(args.output, 'w') as f:
        for _ in range(args.lines):
            line = generate_log_line(timestamp, args.format)
            f.write(line + '\n')

            # Advance time (variable intervals for realism)
            ms_delta = random.randint(10, 5000)
            timestamp += timedelta(milliseconds=ms_delta)

    print(f'Generated {args.lines} lines to {args.output}')

if __name__ == '__main__':
    main()
