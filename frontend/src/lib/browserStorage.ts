function getBrowserStorage() {
  if (typeof window === "undefined") return null;

  try {
    return window.localStorage;
  } catch (error) {
    console.error("Failed to access localStorage:", error);
    return null;
  }
}

export function readStorageValue(key: string) {
  const storage = getBrowserStorage();
  if (!storage) return null;

  try {
    return storage.getItem(key);
  } catch (error) {
    console.error("Failed to read localStorage:", error);
    return null;
  }
}

export function writeStorageValue(key: string, value: string) {
  const storage = getBrowserStorage();
  if (!storage) return;

  try {
    storage.setItem(key, value);
  } catch (error) {
    console.error("Failed to write localStorage:", error);
  }
}

export function removeStorageValue(key: string) {
  const storage = getBrowserStorage();
  if (!storage) return;

  try {
    storage.removeItem(key);
  } catch (error) {
    console.error("Failed to remove localStorage:", error);
  }
}
