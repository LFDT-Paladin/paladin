import i18next from "i18next";
import { RpcEndpoint, RpcMethods } from "./rpcMethods";
import { generatePostReq, returnResponse } from "./common";

export const queryPrivacyGroupByAddress = async (address: string): Promise<any> => {
    const requestPayload = {
      jsonrpc: "2.0",
      id: Date.now(),
      method: RpcMethods.pgroup_getGroupByAddress,
      params: [address],
    };
  
    return <Promise<any>>(
      returnResponse(
        () => fetch(RpcEndpoint, generatePostReq(JSON.stringify(requestPayload))),
        i18next.t("errorQueryingPrivacyGroupByAddress")
      )
    );
  };