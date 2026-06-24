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
 * An immutable byte string that serializes to JSON as lower-case hex with a {@code 0x} prefix,
 * mirroring {@code pldtypes.HexBytes}. Parsing accepts hex with or without the {@code 0x} prefix,
 * in any case.
 */
public final class HexBytes {

  private final byte[] value;

  private HexBytes(byte[] value) {
    this.value = value;
  }

  /** Wraps a copy of the supplied bytes. */
  public static HexBytes wrap(byte[] bytes) {
    if (bytes == null) {
      throw new IllegalArgumentException("bytes must not be null");
    }
    return new HexBytes(bytes.clone());
  }

  /** Parses a hex string (with or without {@code 0x}); an empty string decodes to zero bytes. */
  @JsonCreator
  public static HexBytes fromString(String s) {
    return new HexBytes(Hex.decode(s));
  }

  /** Returns a copy of the underlying bytes. */
  public byte[] toByteArray() {
    return value.clone();
  }

  /** Number of bytes. */
  public int size() {
    return value.length;
  }

  public boolean isEmpty() {
    return value.length == 0;
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
    return o instanceof HexBytes other && Arrays.equals(value, other.value);
  }

  @Override
  public int hashCode() {
    return Arrays.hashCode(value);
  }
}
