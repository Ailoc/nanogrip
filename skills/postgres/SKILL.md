---
description: PostgreSQL database operations - queries, backups, restore, user management, common operations.
metadata:
    nanogrip:
        requires:
            bins:
                - psql
name: postgres
---

# PostgreSQL Skill

Use PostgreSQL for database operations. Default port is 5432.

## Connection

### Connect to database
```bash
# Default connection
psql

# Connect to specific database
psql -d mydb
psql -d postgres://user:pass@localhost:5432/mydb

# Connect to remote database
psql -h hostname -p 5432 -U username -d mydb

# With password file
psql -h hostname -U username -d mydb
```

### Connection options
```bash
# Execute and exit
psql -d mydb -c "SELECT * FROM users"

# Run script file
psql -d mydb -f script.sql

# Quiet mode
psql -q -d mydb -f script.sql

# CSV output
psql -d mydb -c "SELECT * FROM users" -A -F,
```

## Basic Commands

### List databases
```bash
\l
\list
SELECT datname FROM pg_database;
```

### Connect to database
```bash
\c mydb
\connect mydb
```

### List tables
```bash
\dt
\dt *.*        # schema qualified
\dt public.*
```

### Table info
```bash
\d table_name
\d+ table_name
```

### List schemas
```bash
\dn
```

### List users/roles
```bash
\du
\du+
```

## Query Operations

### SELECT queries
```bash
# Simple select
SELECT * FROM table_name;

# Specific columns
SELECT col1, col2 FROM table_name;

# With WHERE
SELECT * FROM users WHERE age > 18;

# Limit
SELECT * FROM users LIMIT 10;

# Order by
SELECT * FROM users ORDER BY created_at DESC;
```

### INSERT
```bash
INSERT INTO table_name (col1, col2) VALUES ('value1', 'value2');
INSERT INTO users (name, email) VALUES ('John', 'john@example.com');
```

### UPDATE
```bash
UPDATE table_name SET col1 = 'value1' WHERE id = 1;
UPDATE users SET age = 25 WHERE name = 'John';
```

### DELETE
```bash
DELETE FROM table_name WHERE id = 1;
DELETE FROM users WHERE status = 'inactive';
```

## Database Management

### Create database
```bash
CREATE DATABASE mydb;
CREATE DATABASE mydb WITH OWNER myuser;
```

### Drop database
```bash
DROP DATABASE mydb;
DROP DATABASE IF EXISTS mydb;
```

### Create table
```bash
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100) UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Alter table
```bash
ALTER TABLE users ADD COLUMN age INT;
ALTER TABLE users DROP COLUMN age;
ALTER TABLE users RENAME TO customers;
ALTER TABLE users ALTER COLUMN name TYPE VARCHAR(200);
```

## User Management

### Create user
```bash
CREATE USER myuser WITH PASSWORD 'password';
CREATE USER myuser WITH PASSWORD 'password' SUPERUSER;
```

### Grant privileges
```bash
GRANT ALL PRIVILEGES ON DATABASE mydb TO myuser;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO myuser;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO myuser;
```

### Revoke privileges
```bash
REVOKE ALL PRIVILEGES ON DATABASE mydb FROM myuser;
```

## Backup and Restore

### Backup database
```bash
# Plain SQL dump
pg_dump mydb > backup.sql
pg_dump -U username -h localhost mydb > backup.sql

# Custom format (compressed)
pg_dump -Fc mydb > backup.dump

# Directory format (parallel)
pg_dump -Fd mydb -f backup_dir
```

### Restore database
```bash
# Restore from SQL dump
psql mydb < backup.sql
psql -U username -d mydb -f backup.sql

# Restore from custom format
pg_restore -d mydb backup.dump

# Create new database and restore
createdb newdb
pg_restore -d newdb backup.dump
```

### Backup specific table
```bash
pg_dump -t mytable mydb > table_backup.sql
```

## Index Operations

### Create index
```bash
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_name ON users(name) WHERE status = 'active';
```

### List indexes
```bash
\di
```

### Drop index
```bash
DROP INDEX idx_users_email;
```

## Transaction Operations

### Begin transaction
```bash
BEGIN;
-- your operations
COMMIT;

-- or rollback
ROLLBACK;
```

## Advanced Queries

### Join tables
```bash
SELECT u.name, o.total
FROM users u
INNER JOIN orders o ON u.id = o.user_id;
```

### Aggregation
```bash
SELECT COUNT(*) FROM users;
SELECT status, COUNT(*) FROM orders GROUP BY status;
SELECT AVG(price) FROM products;
```

### Subquery
```bash
SELECT * FROM users
WHERE id IN (SELECT user_id FROM orders WHERE total > 100);
```

## Performance

### Explain query
```bash
EXPLAIN SELECT * FROM users WHERE email = 'test@example.com';
EXPLAIN ANALYZE SELECT * FROM users WHERE email = 'test@example.com';
```

## psql Meta-commands

### Common meta-commands
```bash
\?              # help
\h              # SQL help
\h SELECT       # help on SELECT
\x              # expanded display
\timing         # show query time
\echo :VAR      # print variable
\set            # list variables
```

### Output formats
```bash
\a              # unaligned mode
\A              # aligned mode
\F '|'          # field separator
\o output.txt   # output to file
\o              # output to stdout
```

### Copy data
```bash
COPY table TO '/tmp/file.csv' CSV HEADER;
COPY table FROM '/tmp/file.csv' CSV;
```
