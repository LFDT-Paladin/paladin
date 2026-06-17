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

package org.lfdt.paladin.sdk.core.abi;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;

/**
 * The kind of an {@link AbiEntry}, mirroring {@code abi.EntryType} from firefly-signer. Serializes to
 * its lower-case JSON token (e.g. {@code "function"}); parsing is case-insensitive.
 */
public enum EntryType {

    /** A function/method of the smart contract. */
    FUNCTION("function"),
    /** The constructor. */
    CONSTRUCTOR("constructor"),
    /** The "receive ether" function. */
    RECEIVE("receive"),
    /** The default function to invoke. */
    FALLBACK("fallback"),
    /** An event the smart contract can emit. */
    EVENT("event"),
    /** An error definition. */
    ERROR("error");

    private final String jsonValue;

    EntryType(String jsonValue) {
        this.jsonValue = jsonValue;
    }

    /** The JSON token for this entry type. */
    @JsonValue
    public String jsonValue() {
        return jsonValue;
    }

    /** Resolves an entry type from its JSON token, case-insensitively. */
    @JsonCreator
    public static EntryType fromJson(String s) {
        if (s != null) {
            for (EntryType t : values()) {
                if (t.jsonValue.equalsIgnoreCase(s)) {
                    return t;
                }
            }
        }
        throw new IllegalArgumentException("unknown ABI entry type: \"" + s + "\"");
    }
}
