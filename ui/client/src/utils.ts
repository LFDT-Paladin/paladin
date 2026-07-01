// Copyright © 2026 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { NavigateOptions, To } from 'react-router-dom';
import React from 'react';
import { IFilter, IPagedResult } from './interfaces';

export const toPagedResult = <T>(results: T[], limit: number): IPagedResult<T> => {
  const hasMore = results.length > limit;
  return {
    items: hasMore ? results.slice(0, limit) : results,
    hasMore,
  };
};

export const CONSTANTS = {
  NAVIGATION_DRAWER_WIDTH: 240
}

export const formatJSONWhenApplicable = (value: unknown) => {
  if (typeof value === 'object' && value !== null) {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return String(value);
    }
  }
  return String(value);
};

export const translateFilters = (filters: IFilter[]) => {
  let result: any = {};

  for (const filter of filters) {
    let entry: any = {
      field: filter.field.name,
      value: filter.value,
    };

    if (filter.caseSensitive === false) {
      entry.caseInsensitive = true;
    }

    let operator = filter.operator;

    switch (operator) {
      case 'contains':
        operator = 'like';
        entry.value = `%${entry.value}%`;
        break;
      case 'startsWith':
        operator = 'like';
        entry.value = `${entry.value}%`;
        break;
      case 'endsWith':
        operator = 'like';
        entry.value = `%${entry.value}`;
        break;
      case 'doesNotContain':
        operator = 'like';
        entry.not = true;
        entry.value = `%${entry.value}%`;
        break;
      case 'doesNotStartWith':
        operator = 'like';
        entry.not = true;
        entry.value = `${entry.value}%`;
        break;
      case 'doesNotEndWith':
        operator = 'like';
        entry.not = true;
        entry.value = `%${entry.value}`;
        break;

      case 'on':
        operator = 'equal';
        break;
      case 'onOrAfter':
        operator = 'gte';
        break;
      case 'onOrBefore':
        operator = 'lte';
        break;
      case 'after':
        operator = 'gt';
        break;
      case 'before':
        operator = 'lt';
        break;
    }

    if (filter.field.type === 'boolean') {
      entry.value = Boolean(entry.value);
    }

    if (filter.field.type === 'timestamp') {
      if (filter.field.isSeconds) {
        entry.value = entry.value / 1000;
      } else if (filter.field.isNanoSeconds) {
        entry.value = entry.value * 1000000;
      }
    }

    let group = result[operator] ?? [];
    group.push(entry);
    result[operator] = group;
  }

  return result;
};

export const isValidUUID = (uuid: string) =>
  /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$/.test(
    uuid
  );

export const isValidHex = (hex: string) =>
  /^(0[xX])?([0-9a-fA-F]{2})+$/.test(hex);

export const encodeHex = (str: string) =>
  '0x' +
  [...new TextEncoder().encode(str)]
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');

// Infer the base path from the current URL
// Assume that the base path is the part of the URL up to the "/ui" segment
export const getBasePath = () => {
  const pathname = window.location.pathname;
  const pathSegments = pathname.split('/');
  for (let i = 0; i < pathSegments.length; i++) {
    if (pathSegments[i] === 'ui') {
      // pathSegments[0] is the empty string, so we need to avoid ending up with //something
      return ('/' + pathSegments.slice(0, i).join('/')).replace(/^\/\/+/, '/');
    }
  }
  return '/';
};

export const getShortHash = (hash: string) => {
  if (hash.length < 16) {
    return hash;
  }
  return `${hash.substring(0, 6)}...${hash.substring(hash.length - 4)}`
};

export const getShortId = (hash: string) => {
  if (hash.length < 16) {
    return hash;
  }
  return `${hash.substring(0, 4)}...${hash.substring(hash.length - 4)}`
};

export const isValidTransactionHash = (value: string) => /^(0x)?[a-fA-F0-9]{64}$/i.test(value);

export const capitalize = (value: string): string =>
  value.charAt(0).toUpperCase() + value.slice(1);

export const isValidAddress = (value: string) => /^0x[a-fA-F0-9]{40}$/.test(value);

export const isValidPrivacyGroupId = (value: string) => /^0x[a-fA-F0-9]{64}$/.test(value);

export const customNavigate = (destination: string, mouseEvent: React.MouseEvent<HTMLElement>, navigate: (to: To, options?: NavigateOptions) => void) => {
  if (mouseEvent.metaKey || mouseEvent.ctrlKey || mouseEvent.button === 1) {
    const newTab = window.open(destination, '_blank');
    if (newTab) {
      newTab.focus();
    }
  } else {
    navigate(destination);
  }
};

type AnyObject = Record<string, any>;

function isObject(item: any): item is AnyObject {
  return item && typeof item === 'object' && !Array.isArray(item);
}

export function deepMerge<T extends AnyObject, U extends AnyObject>(target: T, source: U): T & U {
  const output = { ...target } as any;
  if (!isObject(target) || !isObject(source)) {
    return source as any;
  }
  Object.keys(source).forEach((key) => {
    const targetValue = target[key];
    const sourceValue = source[key];
    if (Array.isArray(targetValue) && Array.isArray(sourceValue)) {
      output[key] = [...targetValue, ...sourceValue];
    } else if (isObject(targetValue) && isObject(sourceValue)) {
      output[key] = deepMerge(targetValue, sourceValue);
    } else {
      output[key] = sourceValue;
    }
  });

  return output;
}

export const isValidPrivacyGroupName = (value: string) =>
  /^[a-zA-Z0-9]([a-zA-Z0-9._-]{0,126}[a-zA-Z0-9])?$/.test(value);

export const isValidPrivacyGroupMemberName = (value: string) =>
  /^(?=.{1,128}$)[a-zA-Z0-9][a-zA-Z0-9._-]*@[a-zA-Z0-9._-]*[a-zA-Z0-9]$/.test(value);