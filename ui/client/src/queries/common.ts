// Copyright © 2024 Kaleido, Inc.
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

export const generatePostReq = (stringBody: string): RequestInit => {
  return {
    body: stringBody,
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    method: "POST",
  };
};

export const returnResponse = async (
  resFn: () => Promise<Response>,
  errorMsg: string,
  ignoreStatuses: number[] = []
) => {
  const res = await resFn();
  if (!res.ok && !ignoreStatuses.includes(res.status)) {
    throw new Error(errorMsg);
  }
  const body = await res.json();
  if (body.error) {
    throw new Error(body.error.message || errorMsg);
  }
  return body.result;
};
