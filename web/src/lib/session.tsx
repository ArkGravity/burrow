import { createContext, useContext } from "react";
import type { Session } from "./api";
export const SessionContext = createContext<{
  session: Session | null;
  reload: () => Promise<void>;
}>({ session: null, reload: async () => {} });
export const useSession = () => useContext(SessionContext);
