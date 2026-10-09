export function passwordRule(message: string) {
  return {
    validator: (_: unknown, value?: string) => {
      // Match the backend's UTF-8 byte limits, including non-ASCII passwords.
      const length = new TextEncoder().encode(value || "").length;
      return !value || (length >= 12 && length <= 256)
        ? Promise.resolve()
        : Promise.reject(new Error(message));
    },
  };
}
