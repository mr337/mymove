import React, { Component } from 'react';
import { Link } from 'react-router-dom';

import { ProgressTimeline, ProgressTimelineStep } from 'shared/ProgressTimeline';
import RadioButton from 'shared/RadioButton';

import PPMPaymentRequestActionBtns from './PPMPaymentRequestActionBtns';
import WizardHeader from '../WizardHeader';

import './Expenses.css';
import { connect } from 'react-redux';
import DocumentsUploaded from './PaymentReview/DocumentsUploaded';

const reviewPagePath = '/ppm-payment-review';
const nextPagePath = '/ppm-expenses';

class ExpensesLanding extends Component {
  state = {
    hasExpenses: '',
  };

  handleRadioChange = event => {
    this.setState({
      [event.target.name]: event.target.value,
    });
  };

  saveAndAddHandler = () => {
    const { history, moveId } = this.props;
    const { hasExpenses } = this.state;
    if (hasExpenses === 'No') {
      return history.push(`/moves/${moveId}${reviewPagePath}`);
    }
    return history.push(`/moves/${moveId}${nextPagePath}`);
  };

  render() {
    const { hasExpenses } = this.state;
    const { history, moveId } = this.props;
    return (
      <>
        <WizardHeader
          title="Expenses"
          right={
            <ProgressTimeline>
              <ProgressTimelineStep name="Weight" completed />
              <ProgressTimelineStep name="Expenses" current />
              <ProgressTimelineStep name="Review" />
            </ProgressTimeline>
          }
        />
        <div className="usa-grid">
          <DocumentsUploaded moveId={moveId} />
        </div>

        <div className="usa-grid expenses-container">
          <h3 className="expenses-header">Do you have any storage or moving expenses?</h3>
          <ul className="expenses-list">
            <li>
              <strong>Storage</strong> expenses are <strong>reimbursable</strong>.
            </li>
            <li>
              Claimable <strong>moving expenses</strong> (such as weighing fees, rental equipment, or tolls){' '}
              <strong>reduce taxes</strong> on your payment.
            </li>
          </ul>
          <Link to="/allowable-expenses">More about expenses</Link>
          <div className="has-expenses-radio-group">
            <RadioButton
              inputClassName="inline_radio"
              labelClassName="inline_radio"
              label="Yes"
              value="Yes"
              name="hasExpenses"
              checked={hasExpenses === 'Yes'}
              onChange={this.handleRadioChange}
            />
            <RadioButton
              inputClassName="inline_radio"
              labelClassName="inline_radio"
              label="No"
              value="No"
              name="hasExpenses"
              checked={hasExpenses === 'No'}
              onChange={this.handleRadioChange}
            />
          </div>
          <PPMPaymentRequestActionBtns
            cancelHandler={() => {}}
            nextBtnLabel="Continue"
            saveAndAddHandler={this.saveAndAddHandler}
            finishLaterHandler={() => history.push('/')}
            submitButtonsAreDisabled={!hasExpenses}
          />
        </div>
      </>
    );
  }
}

function mapStateToProps(state, ownProps) {
  const moveId = ownProps.match.params.moveId;
  return {
    moveId: moveId,
  };
}

export default connect(mapStateToProps)(ExpensesLanding);
