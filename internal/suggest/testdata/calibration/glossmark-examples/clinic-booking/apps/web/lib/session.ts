import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export interface Session {
  token: string;
  patientId: string;
  displayName: string;
}

export function requireSession(): Session {
  const raw = cookies().get("lk_session")?.value;
  if (!raw) redirect("/api/session?next=" + encodeURIComponent("/appointments"));
  return JSON.parse(Buffer.from(raw, "base64url").toString("utf8")) as Session;
}
