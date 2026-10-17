# @gloss verifies impl:StallionProduction
Feature: Stallion production
  Scenario: breed a stallion
    Given a sire and a dam
    When they breed
    Then a stallion is born
