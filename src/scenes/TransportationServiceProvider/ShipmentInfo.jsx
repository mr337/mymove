import React, { Component } from 'react';
import { bindActionCreators } from 'redux';
import { connect } from 'react-redux';
import { Redirect } from 'react-router-dom';
import { get } from 'lodash';
import { NavLink, Link } from 'react-router-dom';
import { reduxForm } from 'redux-form';
import faPlusCircle from '@fortawesome/fontawesome-free-solid/faPlusCircle';
import { titleCase } from 'shared/constants.js';

import LoadingPlaceholder from 'shared/LoadingPlaceholder';
import { MOVE_DOC_TYPE } from 'shared/constants';
import Alert from 'shared/Alert';
import DocumentList from 'shared/DocumentViewer/DocumentList';
import { withContext } from 'shared/AppContext';
import { SwaggerField } from 'shared/JsonSchemaForm/JsonSchemaField';
import {
  getAllShipmentDocuments,
  selectShipmentDocuments,
  generateGBL,
  generateGBLLabel,
} from 'shared/Entities/modules/shipmentDocuments';
import { getAllTariff400ngItems, selectTariff400ngItems } from 'shared/Entities/modules/tariff400ngItems';
import {
  selectSortedShipmentLineItems,
  fetchAndCalculateShipmentLineItems,
} from 'shared/Entities/modules/shipmentLineItems';
import { getAllInvoices } from 'shared/Entities/modules/invoices';
import { getTspForShipment } from 'shared/Entities/modules/transportationServiceProviders';
import { selectStorageInTransits, getStorageInTransitsForShipment } from 'shared/Entities/modules/storageInTransits';
import {
  updatePublicShipment,
  getPublicShipment,
  selectShipment,
  getPublicShipmentLabel,
  acceptShipment,
  acceptPublicShipmentLabel,
  completePmSurvey,
  transportShipment,
  deliverShipment,
  selectShipmentStatus,
} from 'shared/Entities/modules/shipments';
import {
  getServiceAgentsForShipment,
  selectServiceAgentsForShipment,
  updateServiceAgentsForShipment,
} from 'shared/Entities/modules/serviceAgents';

import FontAwesomeIcon from '@fortawesome/react-fontawesome';
import faPhone from '@fortawesome/fontawesome-free-solid/faPhone';
import faComments from '@fortawesome/fontawesome-free-solid/faComments';
import faEmail from '@fortawesome/fontawesome-free-solid/faEnvelope';
import faExternalLinkAlt from '@fortawesome/fontawesome-free-solid/faExternalLinkAlt';
import TspContainer from 'shared/TspPanel/TspContainer';
import Weights from 'shared/ShipmentWeights';
import Dates from 'shared/ShipmentDates';
import LocationsContainer from 'shared/LocationsPanel/LocationsContainer';
import { getLastRequestIsSuccess, getLastRequestIsLoading, getLastError } from 'shared/Swagger/selectors';
import { resetRequests } from 'shared/Swagger/request';
import FormButton from './FormButton';
import CustomerInfo from './CustomerInfo';
import PreApprovalPanel from 'shared/PreApprovalRequest/PreApprovalPanel.jsx';
import StorageInTransitPanel from 'shared/StorageInTransit/StorageInTransitPanel.jsx';
import InvoicePanel from 'shared/Invoice/InvoicePanel.jsx';
import PickupForm from './PickupForm';
import PremoveSurveyForm from './PremoveSurveyForm';
import ServiceAgentForm from './ServiceAgentForm';

import './tsp.scss';

const attachmentsErrorMessages = {
  400: 'An error occurred',
  417: 'Missing data required to generate a Bill of Lading.',
};

class AcceptShipmentPanel extends Component {
  acceptShipment = () => {
    this.props.acceptShipment();
  };

  render() {
    return (
      <div>
        <button className="usa-button-primary" onClick={this.acceptShipment}>
          Accept Shipment
        </button>
      </div>
    );
  }
}

const DeliveryDateFormView = props => {
  const { schema, onCancel, handleSubmit, submitting, valid } = props;

  return (
    <form data-cy="tsp-enter-delivery-form" className="infoPanel-wizard" onSubmit={handleSubmit}>
      <div className="infoPanel-wizard-header">Enter Delivery</div>
      <SwaggerField fieldName="actual_delivery_date" swagger={schema} required />
      <p className="infoPanel-wizard-help">
        After clicking "Done", please upload the <strong>destination docs</strong>. Use the "Upload new document" link
        in the Documents panel at right.
      </p>

      <div className="infoPanel-wizard-actions-container">
        <a className="infoPanel-wizard-cancel" onClick={onCancel}>
          Cancel
        </a>
        <button
          data-cy="tsp-enter-delivery-submit"
          className="usa-button-primary"
          type="submit"
          disabled={submitting || !valid}
        >
          Done
        </button>
      </div>
    </form>
  );
};

const ReferrerQueueLink = props => {
  const pathname = props.history.location.state ? props.history.location.state.referrerPathname : '';
  switch (pathname) {
    case '/queues/new':
      return (
        <NavLink to="/queues/new" activeClassName="usa-current">
          <span>New Shipments Queue</span>
        </NavLink>
      );
    case '/queues/accepted':
      return (
        <NavLink to="/queues/accepted" activeClassName="usa-current">
          <span>Accepted Shipments Queue</span>
        </NavLink>
      );
    case '/queues/approved':
      return (
        <NavLink to="/queues/approved" activeClassName="usa-current">
          <span>Approved Shipments Queue</span>
        </NavLink>
      );
    case '/queues/in_transit':
      return (
        <NavLink to="/queues/in_transit" activeClassName="usa-current">
          <span>In Transit Shipments Queue</span>
        </NavLink>
      );
    case '/queues/delivered':
      return (
        <NavLink to="/queues/delivered" activeClassName="usa-current">
          <span>Delivered Shipments Queue</span>
        </NavLink>
      );
    case '/queues/all':
      return (
        <NavLink to="/queues/all" activeClassName="usa-current">
          <span>All Shipments Queue</span>
        </NavLink>
      );
    default:
      return (
        <NavLink to="/queues/new" activeClassName="usa-current">
          <span>New Shipments Queue</span>
        </NavLink>
      );
  }
};

const DeliveryDateForm = reduxForm({ form: 'deliver_shipment' })(DeliveryDateFormView);

// Action Buttons Conditions
const hasOriginServiceAgent = (serviceAgents = []) => serviceAgents.some(agent => agent.role === 'ORIGIN');
const hasPreMoveSurvey = (shipment = {}) => shipment.pm_survey_completed_at;

class ShipmentInfo extends Component {
  constructor(props) {
    super(props);
    this.assignTspServiceAgent = React.createRef();
  }
  state = {
    redirectToHome: false,
    editTspServiceAgent: false,
  };

  componentDidMount() {
    const shipmentId = this.props.shipmentId;
    this.props
      .getPublicShipment(shipmentId)
      .then(() => {
        this.props.getServiceAgentsForShipment(shipmentId);
        this.props.getTspForShipment(shipmentId);
        this.props.getAllShipmentDocuments(shipmentId);
        this.props.getAllTariff400ngItems(true);
        this.props.fetchAndCalculateShipmentLineItems(shipmentId, this.props.shipmentStatus);
        this.props.getAllInvoices(shipmentId);
        if (this.props.context.flags.sitPanel) {
          this.props.getStorageInTransitsForShipment(shipmentId);
        }
      })
      .catch(err => {
        this.props.history.replace('/');
      });
  }
  componentWillUnmount() {
    this.props.resetRequests();
  }

  acceptShipment = () => {
    return this.props.acceptShipment(this.props.shipment.id);
  };

  generateGBL = () => {
    return this.props.generateGBL(this.props.shipment.id);
  };

  enterPreMoveSurvey = values => {
    this.props.updatePublicShipment(this.props.shipment.id, values).then(() => {
      if (this.props.shipment.pm_survey_completed_at === undefined) {
        this.props.completePmSurvey(this.props.shipment.id);
      }
    });
  };

  editServiceAgents = values => {
    if (values['destination_service_agent']) {
      values['destination_service_agent']['role'] = 'DESTINATION';
    }
    if (values['origin_service_agent']) {
      values['origin_service_agent']['role'] = 'ORIGIN';
    }
    this.props.updateServiceAgentsForShipment(this.props.shipment.id, values);
  };

  transportShipment = values => this.props.transportShipment(this.props.shipment.id, values);

  deliverShipment = values => {
    this.props.deliverShipment(this.props.shipment.id, values).then(() => {
      this.props.fetchAndCalculateShipmentLineItems(this.props.shipment.id, this.props.shipment.status);
      if (this.props.context.flags.sitPanel) {
        this.props.getStorageInTransitsForShipment(this.props.shipment.id);
      }
    });
  };

  render() {
    const {
      context,
      shipment,
      shipmentDocuments,
      generateGBLSuccess,
      generateGBLError,
      generateGBLInProgress,
      serviceAgents,
      loadTspDependenciesHasSuccess,
      gblGenerated,
    } = this.props;
    const { service_member: serviceMember = {}, move = {}, gbl_number: gbl } = shipment;

    const shipmentId = this.props.shipmentId;
    const newDocumentUrl = `/shipments/${shipmentId}/documents/new`;
    const showDocumentViewer = context.flags.documentViewer;
    const showSitPanel = context.flags.sitPanel;
    const awarded = shipment.status === 'AWARDED';
    const accepted = shipment.status === 'ACCEPTED';
    const approved = shipment.status === 'APPROVED';
    const inTransit = shipment.status === 'IN_TRANSIT';
    const delivered = shipment.status === 'DELIVERED';
    const pmSurveyComplete = Boolean(shipment.pm_survey_completed_at);
    const canAssignServiceAgents = (approved || accepted) && !hasOriginServiceAgent(serviceAgents);
    const canEnterPreMoveSurvey =
      (accepted || approved) && hasOriginServiceAgent(serviceAgents) && !hasPreMoveSurvey(shipment);
    const canEnterPackAndPickup = approved && gblGenerated;

    // Some statuses are directly related to the shipment status and some to combo states
    var statusText = 'Unknown status';
    if (awarded) {
      statusText = 'Shipment awarded';
    } else if (accepted) {
      statusText = 'Shipment accepted';
    } else if (approved && !pmSurveyComplete) {
      statusText = 'Awaiting pre-move survey';
    } else if (approved && pmSurveyComplete && !gblGenerated) {
      statusText = 'Pre-move survey complete';
    } else if (approved && pmSurveyComplete && gblGenerated) {
      statusText = 'Outbound';
    } else if (inTransit) {
      statusText = 'Inbound';
    } else if (delivered) {
      statusText = 'Delivered';
    }

    if (this.state.redirectToHome) {
      return <Redirect to="/" />;
    }

    if (!loadTspDependenciesHasSuccess) {
      return <LoadingPlaceholder />;
    }

    return (
      <div>
        <div className="usa-grid grid-wide">
          <div className="usa-width-two-thirds page-title">
            <div className="move-info">
              <div className="move-info-code">
                MOVE INFO &mdash; {move.selected_move_type} CODE {shipment.traffic_distribution_list.code_of_service}
              </div>
              <div className="service-member-name">
                {serviceMember.last_name}, {serviceMember.first_name}
              </div>
            </div>
            <div data-cy="shipment-status" className="shipment-status">
              Status: {statusText}
            </div>
          </div>
          <div className="usa-width-one-third nav-controls">
            <ReferrerQueueLink history={this.props.history} />
          </div>
        </div>
        <div className="usa-grid grid-wide">
          <div className="usa-width-one-whole">
            <ul className="move-info-header-meta">
              <li>
                GBL# {gbl}
                &nbsp;
              </li>
              <li>
                Locator# {move.locator}
                &nbsp;
              </li>
              <li>
                {this.props.shipment.source_gbloc} to {this.props.shipment.destination_gbloc}
                &nbsp;
              </li>
              <li>
                DoD ID# {serviceMember.edipi}
                &nbsp;
              </li>
              <li>
                {serviceMember.telephone}
                {serviceMember.phone_is_preferred && (
                  <FontAwesomeIcon className="icon icon-grey" icon={faPhone} flip="horizontal" />
                )}
                {serviceMember.text_message_is_preferred && (
                  <FontAwesomeIcon className="icon icon-grey" icon={faComments} />
                )}
                {serviceMember.email_is_preferred && <FontAwesomeIcon className="icon icon-grey" icon={faEmail} />}
                &nbsp;
              </li>
            </ul>
          </div>
        </div>
        <div className="usa-grid grid-wide panels-body">
          <div className="usa-width-one-whole">
            <div className="usa-width-two-thirds">
              {awarded && (
                <AcceptShipmentPanel acceptShipment={this.acceptShipment} shipmentStatus={this.props.shipment.status} />
              )}

              {generateGBLError && (
                <p>
                  <Alert
                    type="warning"
                    heading={attachmentsErrorMessages[this.props.generateGBLError.status] || 'An error occurred'}
                  >
                    {titleCase(get(generateGBLError.response, 'body.message', '')) ||
                      'Something went wrong contacting the server.'}
                  </Alert>
                </p>
              )}

              {generateGBLSuccess && (
                <Alert type="success" heading="GBL has been created">
                  <span className="usa-grid usa-alert-no-padding">
                    <span className="usa-width-two-thirds">Click the button to view, print, or download the GBL.</span>
                    <span className="usa-width-one-third">
                      <Link to={`${this.props.gblDocUrl}`} className="usa-alert-right" target="_blank">
                        <button>View GBL</button>
                      </Link>
                    </span>
                  </span>
                </Alert>
              )}
              {pmSurveyComplete &&
                !gblGenerated && (
                  <div>
                    <button onClick={this.generateGBL} disabled={!approved || generateGBLInProgress}>
                      Generate the GBL
                    </button>
                  </div>
                )}
              {canEnterPreMoveSurvey && (
                <FormButton
                  shipmentId={shipmentId}
                  FormComponent={PremoveSurveyForm}
                  schema={this.props.shipmentSchema}
                  onSubmit={this.enterPreMoveSurvey}
                  buttonTitle="Enter pre-move survey"
                />
              )}
              {canAssignServiceAgents && (
                <FormButton
                  shipmentId={shipmentId}
                  serviceAgents={this.props.serviceAgents}
                  FormComponent={ServiceAgentForm}
                  schema={this.props.serviceAgentSchema}
                  onSubmit={this.editServiceAgents}
                  buttonTitle="Assign servicing agents"
                />
              )}

              {inTransit && (
                <FormButton
                  FormComponent={DeliveryDateForm}
                  schema={this.props.deliverSchema}
                  onSubmit={this.deliverShipment}
                  buttonTitle="Enter Delivery"
                  buttonDataCy="tsp-enter-delivery"
                />
              )}
              {canEnterPackAndPickup && (
                <FormButton
                  shipmentId={shipmentId}
                  FormComponent={PickupForm}
                  schema={this.props.transportSchema}
                  onSubmit={this.transportShipment}
                  buttonTitle="Enter Pickup"
                />
              )}
              {this.props.loadTspDependenciesHasSuccess && (
                <div className="office-tab">
                  <Dates title="Dates" shipment={this.props.shipment} update={this.props.updatePublicShipment} />
                  <Weights
                    title="Weights & Items"
                    shipment={this.props.shipment}
                    update={this.props.updatePublicShipment}
                  />
                  <LocationsContainer shipment={this.props.shipment} update={this.props.updatePublicShipment} />
                  <PreApprovalPanel shipmentId={this.props.match.params.shipmentId} />
                  {showSitPanel && <StorageInTransitPanel shipmentId={this.props.shipmentId} />}

                  <TspContainer
                    title="TSP & Servicing Agents"
                    shipment={this.props.shipment}
                    serviceAgents={this.props.serviceAgents}
                    transportationServiceProviderId={this.props.shipment.transportation_service_provider_id}
                  />

                  <InvoicePanel shipmentId={this.props.match.params.shipmentId} shipmentStatus={shipment.status} />
                </div>
              )}
            </div>
            <div className="usa-width-one-third">
              <div className="customer-info">
                <h2 className="extras usa-heading">Customer Info</h2>
                <CustomerInfo shipment={this.props.shipment} />
              </div>
              <div className="documents">
                <h2 className="extras usa-heading">
                  Documents
                  {!showDocumentViewer && <FontAwesomeIcon className="icon" icon={faExternalLinkAlt} />}
                  {showDocumentViewer && (
                    <Link to={newDocumentUrl} target="_blank">
                      <FontAwesomeIcon className="icon" icon={faExternalLinkAlt} />
                    </Link>
                  )}
                </h2>
                <DocumentList
                  detailUrlPrefix={`/shipments/${shipmentId}/documents`}
                  moveDocuments={shipmentDocuments}
                />
                <Link className="status upload-documents-link" to={newDocumentUrl} target="_blank">
                  <span>
                    <FontAwesomeIcon className="icon link-blue" icon={faPlusCircle} />
                  </span>
                  Upload new document
                </Link>
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }
}

const mapStateToProps = (state, props) => {
  const shipmentId = props.match.params.shipmentId;
  const shipment = selectShipment(state, shipmentId);
  const shipmentDocuments = selectShipmentDocuments(state, shipment.id) || {};
  const gbl = shipmentDocuments.find(element => element.move_document_type === MOVE_DOC_TYPE.GBL);
  const gblGenerated = !!gbl;

  return {
    swaggerError: state.swaggerPublic.hasErrored,
    shipment,
    shipmentStatus: selectShipmentStatus(state, shipmentId),
    shipmentDocuments,
    gblGenerated,
    storageInTransits: selectStorageInTransits(state, shipmentId),
    tariff400ngItems: selectTariff400ngItems(state),
    shipmentLineItems: selectSortedShipmentLineItems(state),
    serviceAgents: selectServiceAgentsForShipment(state, shipmentId),
    tsp: get(state, 'tsp'),
    loadTspDependenciesHasSuccess: getLastRequestIsSuccess(state, getPublicShipmentLabel),
    loadTspDependenciesHasError: getLastError(state, getPublicShipmentLabel),
    acceptError: getLastError(state, acceptPublicShipmentLabel),
    generateGBLError: get(state, 'tsp.generateGBLError'),
    generateGBLInProgress: getLastRequestIsLoading(state, generateGBLLabel),
    generateGBLSuccess: getLastRequestIsSuccess(state, generateGBLLabel),
    gblDocUrl: `/shipments/${shipment.id}/documents/${get(gbl, 'id')}`,
    error: get(state, 'tsp.error'),
    shipmentSchema: get(state, 'swaggerPublic.spec.definitions.Shipment', {}),
    serviceAgentSchema: get(state, 'swaggerPublic.spec.definitions.ServiceAgent', {}),
    storageInTransitsSchema: get(state, 'swaggerPublic.spec.definitions.StorageInTransits', {}),
    transportSchema: get(state, 'swaggerPublic.spec.definitions.TransportPayload', {}),
    deliverSchema: get(state, 'swaggerPublic.spec.definitions.ActualDeliveryDate', {}),
    shipmentId,
  };
};

const mapDispatchToProps = dispatch =>
  bindActionCreators(
    {
      getPublicShipment,
      getServiceAgentsForShipment,
      completePmSurvey,
      updatePublicShipment,
      acceptShipment,
      generateGBL,
      updateServiceAgentsForShipment,
      transportShipment,
      deliverShipment,
      getAllShipmentDocuments,
      getAllTariff400ngItems,
      fetchAndCalculateShipmentLineItems,
      getAllInvoices,
      getTspForShipment,
      resetRequests,
      getStorageInTransitsForShipment,
      selectStorageInTransits,
    },
    dispatch,
  );

const connectedShipmentInfo = withContext(connect(mapStateToProps, mapDispatchToProps)(ShipmentInfo));

export { DeliveryDateFormView, connectedShipmentInfo as default, ReferrerQueueLink };
