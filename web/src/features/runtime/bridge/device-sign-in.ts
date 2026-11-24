/** Keep the approval destination in the server-side OIDC flow, including in a new tab. */
export function deviceSignInURL(code: string): string {
  const destination = `/device?${new URLSearchParams({ code })}`;
  return `/auth/login?${new URLSearchParams({ rd: destination })}`;
}
