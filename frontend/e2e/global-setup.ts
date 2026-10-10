import { seed } from "./seed";
import { startStack, stopStack } from "./stack";

/** Boots Postgres + API and seeds them; the returned function tears everything down. */
export default async function globalSetup() {
  try {
    await startStack();
    await seed();
  } catch (error) {
    stopStack();
    throw error;
  }
  return () => stopStack();
}
