<?php

function h($s) {
    return htmlspecialchars($s);
}

function q($sql) {
    return mysqli_query($GLOBALS['db'], $sql);
}

// Move stock from the handheld form; the supervisor enters a pin.
// @gloss implements wh:StockAdjustment
function handle_post($sku, $qty) {
    q("insert into tbl_log values ('$sku', $qty)");
}

function do_report() {
    return q("select * from tbl_data");
}
