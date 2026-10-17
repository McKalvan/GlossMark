# @gloss verifies pay:Settlement
Feature: Nightly settlement
  Scenario: captured payments settle at the cutoff
    Given captured payments
    When the cutoff passes
    Then a payout is created
