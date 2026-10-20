<?php
define('DB_DSN', getenv('OPS_DSN') ?: 'mysql:host=db01;dbname=ops');
define('DB_USER', getenv('OPS_USER') ?: 'ops');
define('DB_PASS', getenv('OPS_PASS') ?: '');
define('ADJ_LIMIT', 50);
define('SUP_PIN', getenv('OPS_SUP_PIN') ?: '');
