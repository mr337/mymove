import React, { Component, Fragment } from 'react';
import Select from 'react-select';
import { reduxForm } from 'redux-form';
import { connect } from 'react-redux';
import { get } from 'lodash';
import PropTypes from 'prop-types';

import carImg from 'shared/images/CAR.png';
import carTrailer from 'shared/images/CAR_TRAILER.png';
import boxTrailer from 'shared/images/BOX_TRUCK.png';
import { ProgressTimeline, ProgressTimelineStep } from 'shared/ProgressTimeline';
import { SwaggerField } from 'shared/JsonSchemaForm/JsonSchemaField';

import PPMPaymentRequestActionBtns from './PPMPaymentRequestActionBtns';
import WizardHeader from '../WizardHeader';
import './PPMPaymentRequest.css';

const vehicleImages = {
  CAR: carImg,
  CAR_TRAILER: carTrailer,
  BOX_TRUCK: boxTrailer,
};

export class WeightTicket extends Component {
  state = { vehicleOptions: '' };

  renderOptions() {
    const { vehicleTypes } = this.props;
    return vehicleTypes.map(({ value, label }) => ({
      value,
      label: (
        <>
          {/* eslint-disable-next-line security/detect-object-injection */}
          <img alt={label} src={vehicleImages[value]} />
          {label}
        </>
      ),
    }));
  }

  onChange = e => {
    this.setState({
      vehicleOptions: e.target.value,
    });
  };

  render() {
    const { vehicleOptions } = this.state;
    const { schema } = this.props;
    return (
      <Fragment>
        <WizardHeader
          title="Weight tickets"
          right={
            <ProgressTimeline>
              <ProgressTimelineStep name="Weight" current />
              <ProgressTimelineStep name="Expenses" />
              <ProgressTimelineStep name="Review" />
            </ProgressTimeline>
          }
        />
        <div className="usa-grid">
          <select className="rounded select">
            <option className="why">WHY</option>
          </select>
          <Select value={vehicleOptions} onChange={this.onChange} options={this.renderOptions()} />
          <SwaggerField fieldName="vehicle_options" swagger={schema} required />
          <SwaggerField fieldName="vehicle_nickname" swagger={schema} required />
          {/* TODO: change onclick handler to go to next page in flow */}
          <PPMPaymentRequestActionBtns onClick={() => {}} nextBtnLabel="Save & Add Another" />
        </div>
      </Fragment>
    );
  }
}

const formName = 'weight_ticket_wizard';
WeightTicket = reduxForm({
  form: formName,
  enableReinitialize: true,
  keepDirtyOnReinitialize: true,
})(WeightTicket);

WeightTicket.propTypes = {
  schema: PropTypes.object.isRequired,
};

function mapStateToProps(state) {
  const schema = get(state, 'swaggerInternal.spec.definitions.WeightTicketPayload', {});
  let displayValues;
  let vehicleTypes = [];

  if (schema.properties && schema.properties.vehicle_options) {
    displayValues = schema.properties.vehicle_options['x-display-value'];
    vehicleTypes = Object.keys(displayValues).map(vehicleType => ({
      // eslint-disable-next-line security/detect-object-injection
      label: displayValues[vehicleType],
      value: vehicleType,
    }));
  }

  const props = {
    schema,
    vehicleTypes,
  };
  return props;
}
export default connect(mapStateToProps)(WeightTicket);
