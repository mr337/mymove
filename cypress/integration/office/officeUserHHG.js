/* global cy */
describe('office user finds the shipment', function() {
  beforeEach(() => {
    cy.signIntoOffice();
  });
  it('office user views hhg moves in queue new moves', function() {
    officeUserViewsMoves();
  });
  it('office user views active hhg moves in queue Approved HHGs', function() {
    officeUserViewsActiveShipment();
  });
  it('office user views delivered hhg moves in queue Delivered HHGs', function() {
    officeUserViewsDeliveredShipment();
  });
  it('office user approves basics for move, cannot approve HHG shipment', function() {
    officeUserApprovesOnlyBasicsHHG();
  });
  it('office user approves basics for move, verifies and approves HHG shipment', function() {
    officeUserApprovesHHG();
  });
});

function officeUserViewsMoves() {
  // Open new moves queue
  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new/);
  });

  // Find move (generated in e2ebasic.go) and open it
  cy.selectQueueItemMoveLocator('RLKBEM');

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/basics/);
  });

  cy.contains('GBL#').should('be.visible');

  cy.get('[data-cy="hhg-tab"]').click();

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/hhg/);
  });
}

function officeUserViewsDeliveredShipment() {
  // Open new moves queue
  cy.patientVisit('/queues/hhg_delivered');
  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/hhg_delivered/);
  });

  // Find move (generated in e2ebasic.go) and open it
  cy.selectQueueItemMoveLocator('SCHNOO');

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/basics/);
  });

  cy.get('[data-cy="hhg-tab"]').click();

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/hhg/);
  });
}

function officeUserViewsActiveShipment() {
  // Open new moves queue
  cy.patientVisit('/queues/hhg_active');
  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/hhg_active/);
  });

  // Find move (generated in e2ebasic.go) and open it
  cy.selectQueueItemMoveLocator('GBLGBL');

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/basics/);
  });

  cy.get('[data-cy="hhg-tab"]').click();

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/hhg/);
  });
}

function officeUserApprovesOnlyBasicsHHG() {
  // Open approved hhg queue
  cy.patientVisit('/queues/new');
  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new/);
  });

  // Find move and open it
  cy.selectQueueItemMoveLocator('BACON6');

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/basics/);
  });

  cy.get('.combo-button').click();

  // Approve basics
  cy
    .get('.combo-button .dropdown')
    .contains('Approve Basics')
    .click();

  cy.get('.combo-button').click();

  cy.get('.status').contains('Approved');

  // Click on HHG tab
  cy.get('[data-cy="hhg-tab"]').click();

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/hhg/);
  });

  // disabled because shipment not yet accepted
  cy.get('.combo-button').click();

  // Disabled because already approved and not delivered
  cy
    .get('.combo-button .dropdown')
    .contains('Approve HHG')
    .should('have.class', 'disabled');

  cy.get('.combo-button').click();

  cy.get('.status').contains('Awarded');
}

function officeUserApprovesHHG() {
  // Open approved hhg queue
  cy.patientVisit('/queues/new');
  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new/);
  });

  // Find move and open it
  cy.selectQueueItemMoveLocator('BACON5');

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/basics/);
  });

  cy.get('.combo-button').click();

  // Approve basics
  cy
    .get('.combo-button .dropdown')
    .contains('Approve Basics')
    .click();

  cy.get('.combo-button').click();

  cy.get('.status').contains('Approved');

  // Click on HHG tab
  cy.get('[data-cy="hhg-tab"]').click();

  cy.location().should(loc => {
    expect(loc.pathname).to.match(/^\/queues\/new\/moves\/[^/]+\/hhg/);
  });

  cy.get('.combo-button').click();

  // Approve HHG

  cy
    .get('.combo-button .dropdown')
    .contains('Approve HHG')
    .click();

  cy.get('.status').contains('Approved');
}
