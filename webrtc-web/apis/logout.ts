export function logout(apiPrefix: string): Promise<void> {
  return fetch(`${apiPrefix}/logout`, { method: "POST" }).then((r) => {
    console.log("Successfully logged out");
  });
}
