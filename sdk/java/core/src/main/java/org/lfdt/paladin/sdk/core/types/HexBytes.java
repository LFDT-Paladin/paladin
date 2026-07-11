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

  private HexBytes(final byte[] value) {
    this.value = value;
  }

  /**
   * Wraps a copy of the supplied bytes.
   *
   * @param bytes the bytes to copy
   * @return a {@code HexBytes} holding a defensive copy of {@code bytes}
   * @throws IllegalArgumentException if {@code bytes} is null
   */
  public static HexBytes wrap(final byte[] bytes) {
    if (bytes == null) {
      throw new IllegalArgumentException("bytes must not be null");
    }
    return new HexBytes(bytes.clone());
  }

  /**
   * Parses a hex string (with or without {@code 0x}); an empty string decodes to zero bytes.
   *
   * @param s the hex string to parse, with or without a {@code 0x} prefix, in any case
   * @return the parsed {@code HexBytes}
   * @throws IllegalArgumentException if {@code s} is not valid hex
   */
  @JsonCreator
  public static HexBytes fromString(final String s) {
    return new HexBytes(Hex.decode(s));
  }

  /**
   * Returns a copy of the underlying bytes.
   *
   * @return a defensive copy of the backing bytes
   */
  public byte[] toByteArray() {
    return value.clone();
  }

  /**
   * Number of bytes.
   *
   * @return the length of the byte string
   */
  public int size() {
    return value.length;
  }

  /**
   * Reports whether the byte string is empty.
   *
   * @return {@code true} if there are no bytes
   */
  public boolean isEmpty() {
    return value.length == 0;
  }

  /**
   * Lower-case hex without a {@code 0x} prefix.
   *
   * @return the bytes as lower-case hex characters (empty string when there are no bytes)
   */
  public String toHex() {
    return Hex.FORMAT.formatHex(value);
  }

  /**
   * Lower-case hex with a {@code 0x} prefix — the JSON representation.
   *
   * @return the bytes as {@code 0x}-prefixed lower-case hex
   */
  @JsonValue
  public String to0xHex() {
    return "0x" + Hex.FORMAT.formatHex(value);
  }

  @Override
  public String toString() {
    return to0xHex();
  }

  @Override
  public boolean equals(final Object o) {
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
