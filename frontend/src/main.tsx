import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { createQueryClient, Root } from "./providers";
import "./index.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Root client={createQueryClient()}>
      <App />
    </Root>
  </StrictMode>,
);
