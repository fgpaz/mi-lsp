class Consumer:
    def use(self, known: "Known", shared: "Shared", missing: "MissingType"):
        return known, shared, missing
