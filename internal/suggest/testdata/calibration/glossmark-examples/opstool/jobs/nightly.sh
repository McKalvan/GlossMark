#!/usr/bin/env bash
# cron: 30 2 * * *  ops  /opt/opstool/jobs/nightly.sh >> /var/log/opstool/nightly.log 2>&1
set -euo pipefail
cd /opt/opstool

python3 - <<'PY'
from core.Manager import Manager
from utils.misc import load_cfg

m = Manager(load_cfg())
print(m.handle_request("repl", {}))
for cutoff in ("07:00", "11:30", "15:00"):
    print(m.handle_request("batch", {"cutoff": cutoff}))
PY

mysqldump --single-transaction ops tbl_cnt tbl_log | gzip > "/backup/ops-$(date +%F).sql.gz"
