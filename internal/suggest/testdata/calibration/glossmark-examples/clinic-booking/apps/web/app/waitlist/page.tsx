import { requireSession } from "@/lib/session";

// The waitlist is designed but not built yet: the page collects interest so
// operations can size it. Nothing in the backend offers freed slots yet.
// @gloss implements .WaitlistPage conf=0.95
export default function WaitlistPage() {
  requireSession();
  return (
    <section>
      <h1>Join the waitlist</h1>
      <p>No time that suits you? Tell us and we will contact you if an earlier slot opens up.</p>
      <form action="/api/waitlist-interest" method="post">
        <label>
          Preferred days
          <input name="days" placeholder="e.g. Tuesday mornings" />
        </label>
        <button className="button" type="submit">Notify me</button>
      </form>
    </section>
  );
}
