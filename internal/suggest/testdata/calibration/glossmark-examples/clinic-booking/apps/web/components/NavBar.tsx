import Link from "next/link";

// @gloss hosts .BookingPage, .AppointmentPage, .PractitionerDirectory conf=0.85
export function NavBar() {
  return (
    <nav className="container" aria-label="Main">
      <Link href="/">Larkspur Clinics</Link>
      {" · "}
      <Link href="/book">Book</Link>
      {" · "}
      <Link href="/appointments">My appointments</Link>
      {" · "}
      <Link href="/practitioners">Practitioners</Link>
    </nav>
  );
}
