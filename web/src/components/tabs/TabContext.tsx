"use client";
import { createContext, useContext, useState, type ReactNode } from "react";

type TabContextValue = {
  tab: string;
  setTab: (t: string) => void;
};

const Ctx = createContext<TabContextValue>({ tab: "positions", setTab: () => {} });

export function TabProvider({ children }: { children: ReactNode }) {
  const [tab, setTab] = useState("positions");
  return <Ctx value={{ tab, setTab }}>{children}</Ctx>;
}

export function useTab() {
  return useContext(Ctx);
}
