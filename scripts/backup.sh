#!/bin/bash
# scripts/backup.sh

BACKUP_DIR="/var/backups/trackr"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# Backup Global DB (Turso - handled by Turso's built-in backups)
# But we export a snapshot for redundancy if using file-based or if CLI tool available
# echo "Exporting global DB snapshot..."
# turso db shell trackr-global ".dump" > $BACKUP_DIR/global_${TIMESTAMP}.sql

# Backup all tenant databases
echo "Backing up tenant databases..."
mkdir -p $BACKUP_DIR/tenants_${TIMESTAMP}
cp /var/lib/trackr/dbs/*.db $BACKUP_DIR/tenants_${TIMESTAMP}/ 2>/dev/null

# Compress backups
echo "Compressing..."
tar -czf $BACKUP_DIR/full_backup_${TIMESTAMP}.tar.gz \
  $BACKUP_DIR/tenants_${TIMESTAMP}

# Cleanup old backups (keep last 30 days)
find $BACKUP_DIR -name "full_backup_*.tar.gz" -mtime +30 -delete

# Upload to S3 (optional)
# aws s3 cp $BACKUP_DIR/full_backup_${TIMESTAMP}.tar.gz s3://trackr-backups/

echo "Backup completed: full_backup_${TIMESTAMP}.tar.gz"
