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

package org.lfdt.paladin.sdk.core.transaction;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * How a transaction was submitted, mirroring {@code pldapi.SubmitMode}. Populated on output only when a
 * transaction is routed via the prepare-transaction flow; the default is {@link #AUTO}. Serializes to its
 * lower-case JSON token; parsing is case-insensitive.
 */
public enum SubmitMode {

    /** Automatically submitted by Paladin. */
    AUTO("auto"),
    /** Results in a prepared transaction that can be downloaded and submitted externally. */
    EXTERNAL("external"),
    /** Just a call — never persisted as a transaction in the database. */
    CALL("call"),
    /** Occurs when writing the prepared transaction back to the database — not persisted. */
    PREPARE("prepare");

    private final String jsonValue;

    SubmitMode(String jsonValue) {
        this.jsonValue = jsonValue;
    }

    /** The JSON token for this submit mode. */
    @JsonValue
    public String jsonValue() {
        return jsonValue;
    }

    /** Resolves a submit mode from its JSON token, case-insensitively. */
    @JsonCreator
    public static SubmitMode fromJson(String s) {
        if (s != null) {
            for (SubmitMode m : values()) {
                if (m.jsonValue.equalsIgnoreCase(s)) {
                    return m;
                }
            }
        }
        throw new IllegalArgumentException("unknown submit mode: \"" + s + "\"");
    }
}
