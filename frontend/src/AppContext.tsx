import React, { createContext, useContext, useState, ReactNode } from "react";
import { MatchOutcome, UploadResponse } from "./types";

// Define the shape of the context state
interface AppContextType {
  upload: UploadResponse | undefined;
  setUpload: (value: UploadResponse | undefined) => void;
  outcome: MatchOutcome | undefined;
  setOutcome: (value: MatchOutcome | undefined) => void;
}

// Create the context with a default value of undefined
const AppContext = createContext<AppContextType | undefined>(undefined);

// Define the props for the provider
interface AppProviderProps {
  children: ReactNode;
  /** Initial state, for tests that render a page halfway through the flow. */
  initialUpload?: UploadResponse;
  initialOutcome?: MatchOutcome;
}

// Provider component
export const AppProvider: React.FC<AppProviderProps> = ({ children, initialUpload, initialOutcome }) => {
  const [upload, setUpload] = useState<UploadResponse | undefined>(initialUpload);
  const [outcome, setOutcome] = useState<MatchOutcome | undefined>(initialOutcome);

  return (
    <AppContext.Provider value={{ upload, setUpload, outcome, setOutcome }}>
      {children}
    </AppContext.Provider>
  );
};

// Custom hook for consuming the context
export const useAppContext = (): AppContextType => {
  const context = useContext(AppContext);
  if (!context) {
    throw new Error("useAppContext must be used within an AppProvider");
  }
  return context;
};
