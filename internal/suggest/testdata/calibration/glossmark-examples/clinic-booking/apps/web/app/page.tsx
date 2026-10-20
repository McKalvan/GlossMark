import Link from "next/link";

export default function HomePage() {
  return (
    <section>
      <h1>Book an appointment</h1>
      <p>Choose a practitioner and a time that suits you. You can cancel online up to 24 hours ahead.</p>
      <Link className="button" href="/book">Find a time</Link>
    </section>
  );
}
