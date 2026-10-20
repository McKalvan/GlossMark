package Riverbend::Notice;
# Renders one overdue notice for one patron, as an email or a printed letter.
use strict;
use warnings;

use MIME::Lite;
use File::Spec;

sub new {
    my ($class, %args) = @_;
    my $self = { %args };
    return bless $self, $class;
}

sub body {
    my ($self) = @_;
    my $first = $self->{loans}[0];
    my $text  = "Dear $first->{first_name} $first->{last_name},\n\n";
    $text .= "Our records show the following items are overdue as of $self->{date}:\n\n";
    for my $loan (@{ $self->{loans} }) {
        $text .= sprintf("  %-14s %-40.40s due %s (%d days late)\n",
            $loan->{barcode}, $loan->{title}, $loan->{due_date}, $loan->{days_late});
    }
    $text .= "\nPlease return or renew them at riverbendlibrary.example or any branch.\n";
    $text .= "Items 60 days overdue are billed at replacement cost.\n\nRiverbend Public Library\n";
    return $text;
}

sub send_email {
    my ($self, $to) = @_;
    my $msg = MIME::Lite->new(
        From    => $self->{config}{notices}{from},
        To      => $to,
        Subject => 'Overdue items - Riverbend Public Library',
        Data    => $self->body,
    );
    $msg->send('smtp', $self->{config}{notices}{smtp_host}, Timeout => 30);
    return 1;
}

sub queue_print {
    my ($self, $branch_id) = @_;
    my $dir  = File::Spec->catdir($self->{config}{notices}{print_queue}, "branch-$branch_id");
    my $file = File::Spec->catfile($dir, sprintf('%s-%d.txt', $self->{date}, $self->{loans}[0]{patron_id}));
    open(my $fh, '>', $file) or die "cannot write $file: $!";
    print {$fh} $self->body;
    close $fh;
    return $file;
}

1;
