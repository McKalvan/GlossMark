# features/authorization.feature
# @gloss verifies pay:Authorization
Feature: Card authorization
  Scenario: a valid card is authorized
    Given a card
    When the merchant authorizes 10.00
    Then a payment exists
