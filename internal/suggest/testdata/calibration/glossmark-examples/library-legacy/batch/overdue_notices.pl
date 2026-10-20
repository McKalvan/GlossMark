#!/usr/bin/perl
# @gloss implements .OverdueNoticeJob scope=file conf=0.85
# overdue_notices.pl - email or print overdue notices at 7, 14 and 28 days.
# Run from batch/bin/run_nightly.sh. Written 2011 (RT#2207), lost-item rule 2014.
use strict;
use warnings;

use FindBin;
use lib "$FindBin::Bin/lib";
use Getopt::Long;
use POSIX qw(strftime);
use YAML::Tiny;
use Riverbend::DB;
use Riverbend::Notice;

my $config_dir = $ENV{CIRC_CONFIG_DIR} || '/etc/circ';
my $dry_run    = 0;
my $today      = strftime('%Y-%m-%d', localtime);
GetOptions('dry-run' => \$dry_run, 'date=s' => \$today) or die "usage: $0 [--dry-run] [--date YYYY-MM-DD]\n";

my $cfg   = YAML::Tiny->read("$config_dir/batch.yaml")->[0];
my $dbh   = Riverbend::DB->connect();
my %stage = map { $_ => 1 } @{ $cfg->{notices}{overdue_days} };

sub overdue_loans {
    my ($dbh, $today) = @_;
    return $dbh->selectall_arrayref(q{
        SELECT l.id, l.patron_id, l.due_date, i.barcode, t.title,
               p.first_name, p.last_name, p.email, p.email_notices, p.home_branch_id,
               (DATE ? - l.due_date) AS days_late
          FROM loans l
          JOIN items i   ON i.id = l.item_id
          JOIN titles t  ON t.id = i.title_id
          JOIN patrons p ON p.id = l.patron_id
         WHERE l.status IN ('ACTIVE','OVERDUE')
           AND l.due_date < DATE ?
         ORDER BY l.patron_id, l.due_date
    }, { Slice => {} }, $today, $today);
}

sub mark_lost {
    my ($dbh, $loan_id) = @_;
    $dbh->do(q{UPDATE loans SET status = 'LOST' WHERE id = ?}, undef, $loan_id);
    $dbh->do(q{SELECT sp_charge_lost_item(?)}, undef, $loan_id);
}

my %by_patron;
for my $loan (@{ overdue_loans($dbh, $today) }) {
    $dbh->do(q{UPDATE loans SET status = 'OVERDUE' WHERE id = ? AND status = 'ACTIVE'}, undef, $loan->{id})
        unless $dry_run;
    if ($loan->{days_late} >= $cfg->{notices}{lost_after_days}) {
        mark_lost($dbh, $loan->{id}) unless $dry_run;
        next;
    }
    next unless $stage{ $loan->{days_late} };
    push @{ $by_patron{ $loan->{patron_id} } }, $loan;
}

my ($emailed, $printed) = (0, 0);
for my $patron_id (sort { $a <=> $b } keys %by_patron) {
    my $loans  = $by_patron{$patron_id};
    my $notice = Riverbend::Notice->new(config => $cfg, loans => $loans, date => $today);
    if ($loans->[0]{email_notices} && $loans->[0]{email}) {
        $notice->send_email($loans->[0]{email}) unless $dry_run;
        $emailed++;
    } else {
        $notice->queue_print($loans->[0]{home_branch_id}) unless $dry_run;
        $printed++;
    }
}

$dbh->commit unless $dry_run;
printf "%s overdue_notices: %d emailed, %d printed%s\n", $today, $emailed, $printed, $dry_run ? ' (dry run)' : '';
exit 0;
