# @gloss verifies lib:HoldFulfillment
Feature: Hold fulfillment
  Scenario: a returned book goes to the next patron in line
    Given a patron holds a book
    When the book is returned
    Then it waits on the pickup shelf for 7 days
