# @gloss verifies lib:Renewal conf=0.95
Feature: Renewal
  Patrons renew from the desk, the OPAC and the kiosks; the same rules apply everywhere.

  Scenario: renew a book with nobody waiting
    Given card "21234000111222" has "31234000456789" due in 2 days
    When the patron renews "31234000456789"
    Then the new due date is 21 days from today

  Scenario: a waiting hold blocks renewal
    Given card "21234000111222" has "31234000456789" due in 2 days
    And another patron has a hold on the same title
    When the patron renews "31234000456789"
    Then the renewal is refused with "ON_HOLD"
