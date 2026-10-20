# Acceptance tests run nightly against the staging desk with Cucumber-JVM (2015).
# @gloss verifies lib:Checkout, lib:PatronBlock, lib:BorrowingLimit, lib:LoanPeriod conf=0.85
Feature: Checkout at the desk
  Staff scan a patron card and an item barcode; CIRC lends the item or says why not.

  Background:
    Given the circulation rules in "config/circulation.yaml"
    And a patron "Ada Okafor" with card "21234000111222" of type "ADULT"

  Scenario: lend a book for three weeks
    Given the book "31234000456789" is on the shelf
    When staff check out "31234000456789" to card "21234000111222"
    Then the loan is due in 21 days

  Scenario: a patron who owes ten dollars is blocked
    Given card "21234000111222" owes $10.00
    When staff check out "31234000456789" to card "21234000111222"
    Then the checkout is refused with "BLOCKED"

  Scenario: juvenile cards stop at the borrowing limit
    Given a patron "Sam Okafor" with card "21234000333444" of type "JUVENILE"
    And card "21234000333444" has 15 items out
    When staff check out "31234000456789" to card "21234000333444"
    Then the checkout is refused with "LIMIT"
