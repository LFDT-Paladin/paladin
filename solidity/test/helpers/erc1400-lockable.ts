import type { Signer } from "ethers";
import { ethers } from "hardhat";
import type { ERC1400Lockable } from "../../typechain-types";

export const ERC1400_CREATE_LOCK_INPUTS_TYPE =
  "tuple(bytes32 partition, uint256 amount, address recipient)";

// ERC-8074 type string — must match ERC1400Lockable.ERC1400LockContentType
export const ERC1400_LOCK_CONTENT_TYPE =
  "ERC1400LockContent(bytes32 partition,uint256 amount,address recipient)";

// Convenience partition ids (32-byte right-padded ASCII).
export const PARTITION_A = ethers.encodeBytes32String("partition-A");
export const PARTITION_B = ethers.encodeBytes32String("partition-B");

export function encodeCreateLockInputs(
  partition: string,
  amount: bigint,
  recipient: string,
): string {
  return ethers.AbiCoder.defaultAbiCoder().encode(
    [ERC1400_CREATE_LOCK_INPUTS_TYPE],
    [{ partition, amount, recipient }],
  );
}

/**
 * Helper to create a lock in an ERC1400Lockable token and get the lock ID.
 * @param token     The ERC1400Lockable contract instance.
 * @param owner     The signer who will own (and initially spend) the lock.
 * @param partition The partition the locked amount is reserved within.
 * @param amount    The amount to lock (in wei).
 * @param recipient The address that will receive tokens when the lock is spent.
 * @returns Promise resolving to the lock ID as a hex string.
 */
export async function createTokenLock(
  token: ERC1400Lockable,
  owner: Signer,
  partition: string,
  amount: bigint,
  recipient: string,
): Promise<string> {
  const createInputs = encodeCreateLockInputs(partition, amount, recipient);

  const tx = await token
    .connect(owner)
    .createLock(createInputs, ethers.ZeroHash, ethers.ZeroHash, "0x");
  const receipt = await tx.wait();

  const lockCreatedEvent = receipt!.logs.find((log) => {
    try {
      const parsed = token.interface.parseLog(log);
      return parsed?.name === "LockCreated";
    } catch {
      return false;
    }
  });

  if (!lockCreatedEvent) {
    throw new Error("LockCreated event not found");
  }

  return token.interface.parseLog(lockCreatedEvent)!.args[0];
}
