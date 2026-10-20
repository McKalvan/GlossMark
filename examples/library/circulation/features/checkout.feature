# @gloss verifies lib:Checkout
Feature: Checkout
  Scenario: a patron borrows a book
    Given a patron with a valid card
    When they check out a book
    Then a loan is due in 21 days
