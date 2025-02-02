import { swaggerRequest } from 'shared/Swagger/request';
import { getPublicClient } from 'shared/Swagger/api';
import { shipmentLineItems as ShipmentLineItemsModel } from '../schema';
import { denormalize } from 'normalizr';
import { get, orderBy, filter, map, keys, flow } from 'lodash';
import { createSelector } from 'reselect';

export const getShipmentLineItemsLabel = 'ShipmentLineItems.getAllShipmentLineItems';
export const createShipmentLineItemLabel = 'ShipmentLineItems.createShipmentLineItem';
export const deleteShipmentLineItemLabel = 'ShipmentLineItems.deleteShipmentLineItem';
export const approveShipmentLineItemLabel = 'ShipmentLineItems.approveShipmentLineItem';
export const updateShipmentLineItemLabel = 'ShipmentLineItems.updateShipmentLineItem';
export const recalculateShipmentLineItemsLabel = 'ShipmentLineItems.recalculateShipmentLineItems';

export function createShipmentLineItem(shipmentId, payload, label = createShipmentLineItemLabel) {
  return swaggerRequest(getPublicClient, 'accessorials.createShipmentLineItem', { shipmentId, payload }, { label });
}

export function updateShipmentLineItem(shipmentLineItemId, payload, label = updateShipmentLineItemLabel) {
  return swaggerRequest(
    getPublicClient,
    'accessorials.updateShipmentLineItem',
    { shipmentLineItemId, payload },
    { label },
  );
}

export function deleteShipmentLineItem(shipmentLineItemId, label = deleteShipmentLineItemLabel) {
  return swaggerRequest(getPublicClient, 'accessorials.deleteShipmentLineItem', { shipmentLineItemId }, { label });
}

export function approveShipmentLineItem(shipmentLineItemId, label = approveShipmentLineItemLabel) {
  return swaggerRequest(getPublicClient, 'accessorials.approveShipmentLineItem', { shipmentLineItemId }, { label });
}

export function getAllShipmentLineItems(shipmentId, label = getShipmentLineItemsLabel) {
  return swaggerRequest(getPublicClient, 'accessorials.getShipmentLineItems', { shipmentId }, { label });
}

export function recalculateShipmentLineItems(shipmentId, label = recalculateShipmentLineItemsLabel) {
  return swaggerRequest(getPublicClient, 'accessorials.recalculateShipmentLineItems', { shipmentId }, { label });
}

export function fetchAndCalculateShipmentLineItems(shipmentId, shipmentStatus) {
  return async function(dispatch) {
    let result = await dispatch(getAllShipmentLineItems(shipmentId));
    if (result.response.ok && shipmentStatus === 'DELIVERED') {
      const lineItems = result.response.body;
      const lineItem = lineItems.find(item => !item.invoice_id);
      if (lineItem) {
        // recalculate shipment line items if no invoice and shipment delivered
        result = dispatch(recalculateShipmentLineItems(shipmentId));
      }
    }

    return result;
  };
}

// Show linehaul (and related) items before any accessorial items by adding isLinehaul property.
function listLinehaulItemsBeforeAccessorials(items) {
  const linehaulRelatedItemsOrderMap = new Map([
    ['LHS', 1],
    ['16A', 2],
    ['135A', 3],
    ['135B', 4],
    ['105A', 5],
    ['105C', 6],
  ]);
  const storageInTransitRelatedItems = ['185A', '185B', '210A', '210B', '210C', '210F'];
  return items.map(item => {
    return {
      ...item,
      isLinehaul: linehaulRelatedItemsOrderMap.has(item.tariff400ng_item.code)
        ? linehaulRelatedItemsOrderMap.get(item.tariff400ng_item.code)
        : 10,
      isStorageInTransit: storageInTransitRelatedItems.includes(item.tariff400ng_item.code) ? 1 : 10,
    };
  });
}

function orderItemsBy(items) {
  const sortOrder = {
    fields: [
      'isLinehaul',
      'status',
      'approved_date',
      'submitted_date',
      'isStorageInTransit',
      'location',
      'tariff400ng_item.code',
    ],
    order: ['asc', 'asc', 'desc', 'desc', 'asc', 'desc', 'asc'],
  };
  return orderBy(items, sortOrder.fields, sortOrder.order);
}

const selectShipmentLineItems = (state, shipmentId) => {
  let filteredItems = denormalize(
    keys(get(state, 'entities.shipmentLineItems', {})),
    ShipmentLineItemsModel,
    state.entities,
  );
  //only filter by shipmentId if it is explicitly passed
  if (!shipmentId) {
    return filteredItems;
  }
  return filterByShipmentId(shipmentId, filteredItems);
};

export const selectSortedShipmentLineItems = createSelector([selectShipmentLineItems], items =>
  flow([listLinehaulItemsBeforeAccessorials, orderItemsBy])(items),
);

export const selectSortedPreApprovalShipmentLineItems = createSelector(
  [selectSortedShipmentLineItems],
  shipmentLineItems => filter(shipmentLineItems, lineItem => lineItem.tariff400ng_item.requires_pre_approval),
);

export const selectShipmentLineItem = (state, id) => denormalize([id], ShipmentLineItemsModel, state.entities)[0];

const selectInvoicesShipmentLineItemsByInvoiceId = (state, invoiceId) => {
  const items = filterByInvoiceId(invoiceId, getShipmentIds(state));
  return denormalize(map(items, 'id'), ShipmentLineItemsModel, getEntities(state));
};

const selectUnbilledShipmentLineItemsByShipmentId = (state, shipmentId) => {
  return flow([
    filterByShipmentId.bind(this, shipmentId),
    filterByNoInvoiceId,
    denormItems.bind(this, getEntities(state)),
    filterByLinehaulOrPreApprovals,
  ])(getShipmentIds(state));
};

export const selectUnbilledShipmentLineItems = createSelector([selectUnbilledShipmentLineItemsByShipmentId], items =>
  flow([listLinehaulItemsBeforeAccessorials, orderItemsBy])(items),
);

export const selectInvoiceShipmentLineItems = createSelector([selectInvoicesShipmentLineItemsByInvoiceId], items =>
  flow([listLinehaulItemsBeforeAccessorials, orderItemsBy])(items),
);

export const selectTotalFromUnbilledLineItems = createSelector([selectUnbilledShipmentLineItemsByShipmentId], items => {
  return items.reduce((acm, item) => {
    return acm + (item.amount_cents ? item.amount_cents : 0);
  }, 0);
});

export const selectTotalFromInvoicedLineItems = createSelector([selectInvoicesShipmentLineItemsByInvoiceId], items => {
  return items.reduce((acm, item) => {
    return acm + item.amount_cents;
  }, 0);
});

export const selectLocationFromTariff400ngItem = (state, selectedTariff400ngItem) => {
  if (!selectedTariff400ngItem) return [];
  const lineItemLocations = get(state, 'swaggerPublic.spec.definitions.ShipmentLineItem', {}).properties.location;
  if (!lineItemLocations.enum) return [];
  const tariff400ngItemLocation = selectedTariff400ngItem.location;
  // Choose location options based on tariff400ng choice.
  return lineItemLocations.enum.filter(lineItemLocation => {
    return tariff400ngItemLocation === 'EITHER'
      ? lineItemLocation === 'ORIGIN' || lineItemLocation === 'DESTINATION'
      : lineItemLocation === tariff400ngItemLocation;
  });
};

function getShipmentIds(state) {
  return get(state, 'entities.shipmentLineItems', {});
}
function getEntities(state) {
  return get(state, 'entities', {});
}
function denormItems(entities, items) {
  return denormalize(map(items, 'id'), ShipmentLineItemsModel, entities);
}

function filterByShipmentId(shipmentId, items) {
  return filter(items, item => item.shipment_id === shipmentId);
}

function filterByInvoiceId(invoiceId, items) {
  return filter(items, item => item.invoice_id === invoiceId);
}

function filterByNoInvoiceId(items) {
  return filter(items, item => !item.invoice_id);
}

function filterByLinehaulOrPreApprovals(items) {
  return filter(
    items,
    item =>
      !item.tariff400ng_item.requires_pre_approval ||
      item.status === 'APPROVED' ||
      item.status === 'CONDITIONALLY_APPROVED',
  );
}
