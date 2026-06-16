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

package org.lfdt.paladin.sdk.core.types;

import com.fasterxml.jackson.annotation.JsonCreator;
import com.fasterxml.jackson.annotation.JsonValue;
import java.util.Arrays;

/**
 * A 20-byte Ethereum address. Serializes to JSON as lower-case hex with a {@code 0x} prefix (40 hex
 * characters), mirroring {@code pldtypes.EthAddress}. Parsing accepts the address with or without the
 * {@code 0x} prefix, in any case.
 */
public final class EthAddress {

    /** Length of an address in bytes. */
    public static final int SIZE = 20;

    private final byte[] value;

    private EthAddress(byte[] value) {
        this.value = value;
    }

    /** Wraps a copy of exactly {@value #SIZE} bytes. */
    public static EthAddress wrap(byte[] bytes) {
        if (bytes == null || bytes.length != SIZE) {
            throw new IllegalArgumentException(
                    "EthAddress requires exactly " + SIZE + " bytes, got " + (bytes == null ? "null" : bytes.length));
        }
        return new EthAddress(bytes.clone());
    }

    /** Parses a 20-byte address from hex (with or without {@code 0x}). */
    @JsonCreator
    public static EthAddress fromString(String s) {
        byte[] bytes = Hex.decode(s);
        if (bytes.length != SIZE) {
            throw new IllegalArgumentException(
                    "EthAddress requires " + SIZE + " bytes (" + (SIZE * 2) + " hex chars), got " + bytes.length + " bytes");
        }
        return new EthAddress(bytes);
    }

    /** Returns a copy of the underlying bytes. */
    public byte[] toByteArray() {
        return value.clone();
    }

    /** True if every byte is zero. */
    public boolean isZero() {
        for (byte b : value) {
            if (b != 0) {
                return false;
            }
        }
        return true;
    }

    /** Lower-case hex without a {@code 0x} prefix. */
    public String toHex() {
        return Hex.FORMAT.formatHex(value);
    }

    /** Lower-case hex with a {@code 0x} prefix — the JSON representation. */
    @JsonValue
    public String to0xHex() {
        return "0x" + Hex.FORMAT.formatHex(value);
    }

    @Override
    public String toString() {
        return to0xHex();
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) {
            return true;
        }
        return o instanceof EthAddress other && Arrays.equals(value, other.value);
    }

    @Override
    public int hashCode() {
        return Arrays.hashCode(value);
    }
}
