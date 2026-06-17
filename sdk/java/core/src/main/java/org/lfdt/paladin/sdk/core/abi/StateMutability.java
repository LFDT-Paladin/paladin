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
 * How a function interacts with blockchain state, mirroring {@code abi.StateMutability} from
 * firefly-signer. Serializes to its lower-case JSON token (e.g. {@code "nonpayable"}); parsing is
 * case-insensitive.
 */
public enum StateMutability {

    /** Specified not to read blockchain state. */
    PURE("pure"),
    /** Specified not to modify the blockchain state (read-only). */
    VIEW("view"),
    /** The function accepts ether. */
    PAYABLE("payable"),
    /** The function does not accept ether. */
    NONPAYABLE("nonpayable");

    private final String jsonValue;

    StateMutability(String jsonValue) {
        this.jsonValue = jsonValue;
    }

    /** The JSON token for this state mutability. */
    @JsonValue
    public String jsonValue() {
        return jsonValue;
    }

    /** Resolves a state mutability from its JSON token, case-insensitively. */
    @JsonCreator
    public static StateMutability fromJson(String s) {
        if (s != null) {
            for (StateMutability m : values()) {
                if (m.jsonValue.equalsIgnoreCase(s)) {
                    return m;
                }
            }
        }
        throw new IllegalArgumentException("unknown ABI state mutability: \"" + s + "\"");
    }
}
