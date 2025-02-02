import { get, isEmpty } from 'lodash';
import React, { Component, Fragment } from 'react';
import { connect } from 'react-redux';
import { push } from 'react-router-redux';
import { bindActionCreators } from 'redux';
import {
  selectReimbursement,
  approveReimbursement,
  selectPPMForMove,
  downloadPPMAttachments,
  downloadPPMAttachmentsLabel,
} from 'shared/Entities/modules/ppms';
import { selectAllDocumentsForMove } from 'shared/Entities/modules/moveDocuments';
import { getLastError } from 'shared/Swagger/selectors';

import { no_op } from 'shared/utils';
import { formatCents, formatDate } from 'shared/formatters';
import Alert from 'shared/Alert';
import ToolTip from 'shared/ToolTip';

import FontAwesomeIcon from '@fortawesome/react-fontawesome';
import faCheck from '@fortawesome/fontawesome-free-solid/faCheck';
import faClock from '@fortawesome/fontawesome-free-solid/faClock';
import faPlusSquare from '@fortawesome/fontawesome-free-solid/faPlusSquare';
import faMinusSquare from '@fortawesome/fontawesome-free-solid/faMinusSquare';

import './PaymentsPanel.css';
import { getSignedCertification } from 'shared/Entities/modules/signed_certifications';
import { selectPaymentRequestCertificationForMove } from 'shared/Entities/modules/signed_certifications';
import { selectShipmentForMove } from 'shared/Entities/modules/shipments';

const attachmentsErrorMessages = {
  422: 'Encountered an error while trying to create attachments bundle: Document is in the wrong format',
  424: 'Could not find any receipts or documents for this PPM',
  500: 'An unexpected error has occurred',
};

export function sswIsDisabled(ppm, signedCertification, shipment) {
  return (
    missingSignature(signedCertification) || missingNetWeightOrActualMoveDate(ppm) || isComboAndNotDelivered(shipment)
  );
}

function missingSignature(signedCertification) {
  return isEmpty(signedCertification) || signedCertification.certification_type !== 'PPM_PAYMENT';
}

function missingNetWeightOrActualMoveDate(ppm) {
  return isEmpty(ppm) || !ppm.net_weight || !ppm.actual_move_date;
}

function isComboAndNotDelivered(shipment) {
  return !isEmpty(shipment) && shipment.status !== 'DELIVERED';
}

function getUserDate() {
  return new Date().toISOString().split('T')[0];
}

class PaymentsTable extends Component {
  state = {
    showPaperwork: false,
    disableDownload: false,
  };

  componentDidMount() {
    const { moveId } = this.props;
    if (moveId != null) {
      this.props.getSignedCertification(moveId);
    }
  }

  approveReimbursement = () => {
    this.props.approveReimbursement(this.props.advance.id);
  };

  togglePaperwork = () => {
    this.setState({ showPaperwork: !this.state.showPaperwork });
  };

  disableDownloadAll = () => {
    return this.props.moveDocuments.length < 1;
  };

  startDownload = docTypes => {
    this.setState({ disableDownload: true });
    this.props.downloadPPMAttachments(this.props.ppm.id, docTypes).then(response => {
      const {
        response: {
          obj: { url },
        },
      } = response;
      if (url) {
        // Taken from https://mathiasbynens.github.io/rel-noopener/
        let win = window.open();
        // win can be null if a pop-up blocker is used
        if (win) {
          win.opener = null;
          win.location = url;
        }
      }
      this.setState({ disableDownload: false });
    });
  };

  documentUpload = () => {
    const { moveId } = this.props;
    this.props.push(`/moves/${moveId}/documents/new?move_document_type=SHIPMENT_SUMMARY`);
  };

  downloadShipmentSummary = () => {
    const { moveId } = this.props;
    const userDate = getUserDate();

    // eslint-disable-next-line
    window.open(`/internal/moves/${moveId}/shipment_summary_worksheet/?preparationDate=${userDate}`);
  };

  renderAdvanceAction = () => {
    if (this.props.ppm.status === 'APPROVED') {
      if (this.props.advance.status === 'APPROVED') {
        return <div>{/* Further actions to come*/}</div>;
      } else {
        return (
          <div onClick={this.approveReimbursement}>
            <ToolTip disabled={false} text="Approve" textStyle="tooltiptext-small">
              <FontAwesomeIcon aria-hidden className="icon approval-ready" icon={faCheck} title="Approve" />
            </ToolTip>
          </div>
        );
      }
    } else {
      return (
        <ToolTip
          disabled={false}
          text={"Can't approve payment until shipment is approved"}
          textStyle="tooltiptext-medium"
        >
          <FontAwesomeIcon
            aria-hidden
            className="icon approval-blocked"
            icon={faCheck}
            title="Can't approve payment until shipment is approved."
          />
        </ToolTip>
      );
    }
  };

  render() {
    const attachmentsError = this.props.attachmentsError;
    const advance = this.props.advance;
    const paperworkIcon = this.state.showPaperwork ? faMinusSquare : faPlusSquare;

    return (
      <div className="payment-panel">
        <div className="payment-panel-title">Payments</div>
        <table className="payment-table">
          <tbody>
            <tr>
              <th className="payment-table-column-title" />
              <th className="payment-table-column-title">Amount</th>
              <th className="payment-table-column-title">Disbursement</th>
              <th className="payment-table-column-title">Requested on</th>
              <th className="payment-table-column-title">Status</th>
              <th className="payment-table-column-title">Actions</th>
            </tr>
            {!isEmpty(advance) ? (
              <React.Fragment>
                <tr>
                  <th className="payment-table-subheader" colSpan="6">
                    Payments against PPM Incentive
                  </th>
                </tr>
                <tr>
                  <td className="payment-table-column-content">Advance</td>
                  <td className="payment-table-column-content">
                    ${formatCents(get(advance, 'requested_amount')).toLocaleString()}
                  </td>
                  <td className="payment-table-column-content">{advance.method_of_receipt}</td>
                  <td className="payment-table-column-content">{formatDate(advance.requested_date)}</td>
                  <td className="payment-table-column-content">
                    {advance.status === 'APPROVED' ? (
                      <div>
                        <FontAwesomeIcon aria-hidden className="icon approval-ready" icon={faCheck} title="Approved" />{' '}
                        Approved
                      </div>
                    ) : (
                      <div>
                        <FontAwesomeIcon
                          aria-hidden
                          className="icon approval-waiting"
                          icon={faClock}
                          title="Awaiting Review"
                        />{' '}
                        Awaiting review
                      </div>
                    )}
                  </td>
                  <td className="payment-table-column-content">{this.renderAdvanceAction()}</td>
                </tr>
              </React.Fragment>
            ) : (
              <tr>
                <th className="payment-table-subheader">No payments requested</th>
              </tr>
            )}
          </tbody>
        </table>

        <div className="paperwork">
          <a onClick={this.togglePaperwork}>
            <FontAwesomeIcon aria-hidden className="icon" icon={paperworkIcon} />
            Create payment paperwork
          </a>
          {this.state.showPaperwork && (
            <Fragment>
              {attachmentsError && (
                <Alert type="error" heading="An error occurred">
                  {attachmentsErrorMessages[attachmentsError.statusCode] ||
                    'Something went wrong contacting the server.'}
                </Alert>
              )}
              <p>Complete the following steps in order to generate and file paperwork for payment:</p>
              <div className="paperwork">
                <div className="paperwork-step">
                  <div>
                    <p>Download Shipment Summary Worksheet</p>
                    <p>Download and complete the worksheet, which is a fill-in PDF form.</p>
                  </div>
                  <button disabled={this.props.disableSSW} onClick={this.downloadShipmentSummary}>
                    Download Worksheet (PDF)
                  </button>
                </div>

                <hr />

                <div className="paperwork-step">
                  <div>
                    <p>Download All Attachments (PDF)</p>
                    <p>Download bundle of PPM receipts and attach it to the completed Shipment Summary Worksheet.</p>
                  </div>
                  <button
                    disabled={this.state.disableDownload || this.disableDownloadAll()}
                    onClick={() =>
                      this.startDownload(['OTHER', 'WEIGHT_TICKET', 'WEIGHT_TICKET_SET', 'STORAGE_EXPENSE', 'EXPENSE'])
                    }
                  >
                    Download All Attachments (PDF)
                  </button>
                </div>

                <hr />

                <div className="paperwork-step">
                  <div>
                    <p>Download Orders and Weight Tickets (PDF)</p>
                    <p>
                      Download bundle of Orders and Weight Tickets (without receipts) and attach it to the completed
                      Shipment Summary Worksheet.
                    </p>
                  </div>
                  <button
                    disabled={this.state.disableDownload}
                    onClick={() =>
                      this.startDownload(['OTHER', 'WEIGHT_TICKET', 'WEIGHT_TICKET_SET', 'STORAGE_EXPENSE'])
                    }
                  >
                    Download Orders and Weight Tickets (PDF)
                  </button>
                </div>

                <hr />

                <div className="paperwork-step">
                  <div>
                    <p>Upload completed packet</p>
                    <p>
                      Save the worksheet and attachments together as one PDF. Then upload the completed packet for
                      customer and Finance.
                    </p>
                  </div>
                  <button onClick={this.documentUpload}>Upload Completed Packet</button>
                </div>
              </div>
            </Fragment>
          )}
        </div>
      </div>
    );
  }
}

const mapStateToProps = (state, ownProps) => {
  const { moveId } = ownProps;
  const ppm = selectPPMForMove(state, moveId);
  const shipment = selectShipmentForMove(state, moveId);
  const advance = selectReimbursement(state, ppm.advance);
  const signedCertifications = selectPaymentRequestCertificationForMove(state, moveId);
  const disableSSW = sswIsDisabled(ppm, signedCertifications, shipment);
  const moveDocuments = selectAllDocumentsForMove(state, moveId);
  return {
    ppm,
    disableSSW,
    moveId,
    advance,
    attachmentsError: getLastError(state, `${downloadPPMAttachmentsLabel}-${moveId}`),
    moveDocuments,
  };
};

const mapDispatchToProps = dispatch =>
  bindActionCreators(
    {
      getSignedCertification,
      approveReimbursement,
      update: no_op,
      downloadPPMAttachments,
      push,
    },
    dispatch,
  );

export default connect(mapStateToProps, mapDispatchToProps)(PaymentsTable);
