import { format, parseISO } from "date-fns";
import { book } from "@/app/book/actions";
import type { Slot } from "@/lib/types";

export function SlotPicker({ slots, patientId }: { slots: Slot[]; patientId: string }) {
  if (slots.length === 0) {
    return <p>No open times in the next two weeks. <a href="/waitlist">Join the waitlist</a>.</p>;
  }
  return (
    <form action={book}>
      <input type="hidden" name="patientId" value={patientId} />
      <fieldset>
        <legend>Available times</legend>
        {slots.map((s) => (
          <label key={s.id} className="card">
            <input type="radio" name="slotId" value={s.id} required />
            {format(parseISO(s.startsAt), "EEE d MMM, HH:mm")}
          </label>
        ))}
      </fieldset>
      <label>
        Reason for visit (optional)
        <textarea name="reason" maxLength={500} />
      </label>
      <button className="button" type="submit">Book</button>
    </form>
  );
}
