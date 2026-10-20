<?php require_once __DIR__ . '/functions.php'; ?>
<!doctype html>
<html>
<head><title>ops</title><script src="misc.js"></script></head>
<body>
<h3>Batch <?= h($_GET['b'] ?? '') ?></h3>
<?= process_data($_GET['b'] ?? '') ?>
<input id="scan" autofocus onkeyup="doIt(event)">
<div id="msg"></div>
</body>
</html>
