import { NextResponse, type NextRequest } from "next/server";

// Redirects to the clinic identity provider; the callback sets lk_session.
export function GET(req: NextRequest) {
  const next = req.nextUrl.searchParams.get("next") ?? "/";
  const authorize = new URL(`${process.env.IDP_URL}/authorize`);
  authorize.searchParams.set("client_id", process.env.IDP_CLIENT_ID ?? "larkspur-web");
  authorize.searchParams.set("redirect_uri", `${process.env.PUBLIC_URL}/api/session/callback`);
  authorize.searchParams.set("state", next);
  return NextResponse.redirect(authorize);
}
