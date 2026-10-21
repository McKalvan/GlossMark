export interface HoldProps { holdId: string }

export function PlaceHold({ holdId }: HoldProps) {
  return (
    <section>
      <h2>Hold queue</h2>
      <button title="Place hold">Place hold</button>
    </section>
  );
}
