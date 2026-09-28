export const can = (permissions: string[], permission: string) =>
  permissions.includes("*") || permissions.includes(permission);
export function safeRedirect(value?: string): string {
  if (
    !value ||
    !value.startsWith("/") ||
    value.startsWith("//") ||
    /[\\\r\n]/.test(value)
  )
    return "/";
  return value;
}
