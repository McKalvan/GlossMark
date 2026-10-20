package Riverbend::DB;
# One DBI handle for the batch jobs. Credentials come from the environment
# (set by run_nightly.sh from /etc/circ/batch.env, or by the ECS task since 2024).
use strict;
use warnings;

use DBI;

sub connect {
    my ($class) = @_;
    my $dsn = sprintf('dbi:Pg:dbname=%s;host=%s;port=%s',
        $ENV{CIRC_DB_NAME} || 'circ', $ENV{CIRC_DB_HOST} || 'localhost', $ENV{CIRC_DB_PORT} || 5432);
    my $dbh = DBI->connect($dsn, $ENV{CIRC_DB_USER} || 'circ_batch', $ENV{CIRC_DB_PASSWORD},
        { RaiseError => 1, AutoCommit => 0, pg_enable_utf8 => 1 })
        or die "cannot connect: $DBI::errstr";
    return $dbh;
}

1;
