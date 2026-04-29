import type { ITransaction } from "./transaction";

export interface IDomainConfig {
  signingAlgorithms?: Record<string, number>;
}

export interface IDomain {
  name: string;
  registryAddress: string;
  config?: IDomainConfig;
}

export interface IContractConfig {
  contractConfig?: object;
}

export interface IDomainSmartContract {
  domainName: string;
  domainAddress: string;
  address: string;
  config?: IContractConfig;
}

export interface IDomainSmartContractWithDeployTransaction
  extends IDomainSmartContract {
  deployTransaction?: ITransaction;
}
