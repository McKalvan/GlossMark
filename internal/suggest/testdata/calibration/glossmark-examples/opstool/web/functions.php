<?php
// functions.php -- shared by every page in web/. Do not rename (bookmarks).

require_once __DIR__ . '/config.php';

function db() {
    static $pdo = null;
    if ($pdo === null) {
        $pdo = new PDO(DB_DSN, DB_USER, DB_PASS, [PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION]);
    }
    return $pdo;
}

function q($sql, $args = []) {
    $st = db()->prepare($sql);
    $st->execute($args);
    return $st->fetchAll(PDO::FETCH_ASSOC);
}

function h($s) {
    return htmlspecialchars((string)$s, ENT_QUOTES, 'UTF-8');
}

// @gloss implements .PickListPage conf=0.8
function process_data($batch) {
    $rows = q("SELECT id, loc, item, qty FROM tbl_queue WHERE batch = ? AND kind = 'P' AND done = 0 ORDER BY loc", [$batch]);
    $html = "<table class='pl'><tr><th>Loc</th><th>Item</th><th>Qty</th><th></th></tr>";
    foreach ($rows as $r) {
        $html .= "<tr><td>" . h($r['loc']) . "</td><td>" . h($r['item']) . "</td><td>" . h($r['qty']) . "</td><td>[ ]</td></tr>";
    }
    return $html . "</table>";
}

function do_report($from, $to) {
    $rows = q("SELECT loc, item, found, sys_qty, diff, ts FROM tbl_cnt WHERE ts BETWEEN ? AND ? AND ABS(diff) > ? ORDER BY ABS(diff) DESC", [$from, $to, 0.02]);
    $out = [];
    foreach ($rows as $r) {
        $out[] = sprintf("%-10s %-14s %6d %6d %7.2f%%", $r['loc'], $r['item'], $r['found'], $r['sys_qty'], $r['diff'] * 100);
    }
    return implode("\n", $out);
}

// @gloss implements .ScanConfirm, wh:StockAdjustment conf=0.8
function handle_post() {
    $in = json_decode(file_get_contents('php://input'), true);
    switch ($_GET['a'] ?? '') {
        case 'conf':
            q("UPDATE tbl_queue SET done = 1, qty_done = ? WHERE id = ?", [$in['qty'], $in['id']]);
            q("UPDATE tbl_data d JOIN tbl_queue x ON x.loc = d.loc AND x.item = d.item SET d.qty = d.qty - x.qty WHERE x.id = ?", [$in['id']]);
            $next = q("SELECT id, loc, item, qty FROM tbl_queue WHERE batch = (SELECT batch FROM tbl_queue WHERE id = ?) AND done = 0 ORDER BY loc LIMIT 1", [$in['id']]);
            return ['next' => $next ? $next[0] : null];
        case 'adj':
            if (abs((int)$in['qty']) > ADJ_LIMIT && ($in['pin'] ?? '') !== SUP_PIN) {
                return ['ok' => false, 'err' => 'pin'];
            }
            q("UPDATE tbl_data SET qty = qty + ? WHERE loc = ? AND item = ?", [$in['qty'], $in['loc'], $in['item']]);
            return ['ok' => true];
    }
    return ['ok' => false];
}

if ($_SERVER['REQUEST_METHOD'] === 'POST' && basename($_SERVER['SCRIPT_FILENAME']) === 'functions.php') {
    header('Content-Type: application/json');
    echo json_encode(handle_post());
}
