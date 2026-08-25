/*
 * Copyright contributors to Paladin, an LFDT project
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package org.lfdt.paladin.sdk.core.transport;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * The kind of payload carried by a reliable message between nodes. Serializes to its JSON token;
 * parsing is case-insensitive.
 */
public enum ReliableMessageType {

  /** A private state being distributed to another node. */
  STATE("state"),
  /** A transaction receipt being delivered to another node. */
  RECEIPT("receipt"),
  /** A public transaction submission record. */
  PUBLIC_TRANSACTION_SUBMISSION("public_transaction_submission"),
  /** Sequencing activity shared between nodes. */
  SEQUENCING_ACTIVITY("sequencing_activity"),
  /** A prepared transaction being distributed. */
  PREPARED_TRANSACTION("prepared_txn"),
  /** A privacy group being distributed. */
  PRIVACY_GROUP("privacy_group"),
  /** A privacy group message being distributed. */
  PRIVACY_GROUP_MESSAGE("privacy_group_message");

  private final String jsonValue;

  ReliableMessageType(final String jsonValue) {
    this.jsonValue = jsonValue;
  }

  /**
   * The JSON token for this message type.
   *
   * @return the JSON token
   */
  @JsonValue
  public String jsonValue() {
    return jsonValue;
  }

  /**
   * Resolves a message type from its JSON token, case-insensitively.
   *
   * @param s the JSON token to resolve
   * @return the matching message type
   * @throws IllegalArgumentException if {@code s} is null or not a known message type
   */
  @JsonCreator
  public static ReliableMessageType fromJson(final String s) {
    if (s != null) {
      for (ReliableMessageType t : values()) {
        if (t.jsonValue.equalsIgnoreCase(s)) {
          return t;
        }
      }
    }
    throw new IllegalArgumentException("unknown reliable message type: " + s);
  }
}
