# @gloss verifies lib:OverdueNotification conf=0.86
Feature: Overdue notices
  The nightly batch emails or prints a notice at 7, 14 and 28 days overdue.

  Scenario: first notice by email
    Given card "21234000111222" prefers email notices
    And "31234000456789" was due 7 days ago
    When the nightly batch runs
    Then one email notice is sent to the patron
    And the notice lists "31234000456789"

  Scenario: patrons without email get a printed notice
    Given card "21234000555666" has no email address
    And "31234000456790" was due 14 days ago
    When the nightly batch runs
    Then a printed notice is queued for the Main branch mail run
