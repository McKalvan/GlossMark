# @gloss verifies lib:HoldTrapping, lib:HoldExpiry conf=0.85
Feature: Holds
  Title-level holds are filled first come, first served, and expire when not picked up.

  Scenario: a returned copy is trapped for the oldest hold
    Given patrons "A" and "B" placed holds on title 77 in that order
    When a copy of title 77 is checked in at Main
    Then the copy is trapped for patron "A"
    And the hold slip shows a pickup-by date 7 days away

  Scenario: an uncollected hold expires
    Given a hold trapped 8 days ago and not picked up
    When the nightly batch runs
    Then the hold is "EXPIRED"
    And the copy is trapped for the next waiting hold, or returned to the shelf
