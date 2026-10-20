# @gloss verifies circ:BookIntake
Feature: Book intake
  Scenario: catalog a book
    Given an ISBN and a copy count
    When they are cataloged
    Then a book is on the shelf
