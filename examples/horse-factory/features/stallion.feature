# @gloss verifies impl:StallionProduction
Feature: Stallion production
  Scenario: a sire and a dam produce a stallion
    Given a sire and a dam
    When they breed
    Then a stallion is born
