import {
  truncateNumber,
  addCommasToNumberString,
  formatFromBaseQuantity,
  convertFromBaseQuantity,
  formatBaseQuantityAsDollars,
} from 'shared/formatters';
import { isRobustAccessorial } from 'shared/PreApprovalRequest/DetailsHelper';

export const displayBaseQuantityUnits = (item, scale) => {
  if (!item) return;

  const itemCode = item.tariff400ng_item.code;
  const itemQuantity1 = item.quantity_1;
  const itemQuantity2 = item.quantity_2;

  if (isWeight(itemCode)) {
    const decimalPlaces = 0;
    const weight = convertTruncateAddCommas(itemQuantity1, decimalPlaces);
    return `${weight} lbs`;
  } else if (isWeightDistance(itemCode)) {
    const decimalPlaces = 0;
    const weight = convertTruncateAddCommas(itemQuantity1, decimalPlaces);
    const milage = convertTruncateAddCommas(itemQuantity2, decimalPlaces);
    return `${weight} lbs, ${milage} mi`;
  } else if (isVolume(itemCode) && isRobustAccessorial(item)) {
    const decimalPlaces = 2;
    const volume = convertTruncateAddCommas(itemQuantity1, decimalPlaces);
    return `${volume} cu ft`;
  } else if (isPrice(itemCode) && isRobustAccessorial(item)) {
    const price = formatBaseQuantityAsDollars(itemQuantity1);
    return `$${price}`;
  } else if (isDistance(itemCode)) {
    const decimalPlaces = 0;
    const milage = convertTruncateAddCommas(itemQuantity1, decimalPlaces);
    return `${milage} mi`;
  }

  return formatFromBaseQuantity(itemQuantity1);
};

function isWeight(itemCode) {
  const lbsItems = ['105A', '105C', '135A', '135B'];
  return lbsItems.includes(itemCode);
}

function isVolume(itemCode) {
  const cuFtItems = ['105B', '105E'];
  return cuFtItems.includes(itemCode);
}

function isWeightDistance(itemCode) {
  const lbsMiItems = ['LHS', '16A'];
  return lbsMiItems.includes(itemCode);
}

function isPrice(itemCode) {
  const priceItems = ['226A', '35A'];
  return priceItems.includes(itemCode);
}

function isDistance(itemCode) {
  const miItems = ['210A', '210B', '210C'];
  return miItems.includes(itemCode);
}

function convertTruncateAddCommas(value, decimalPlaces) {
  const convertedValue = convertFromBaseQuantity(value);
  const formattedValue = truncateNumber(convertedValue, decimalPlaces);
  return addCommasToNumberString(formattedValue, decimalPlaces);
}
