package b2c

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

var ErrInvalidListingPrice = errors.New("asking_manwon must be an integer between 1 and 2147483647")

// UpdatePrice changes only the registrant's active directly registered listing.
// The database notification is emitted only after the write commits.
func (s *ListingStore) UpdatePrice(ctx context.Context, id string, price int, registeredBy string) error {
	if price <= 0 || price > 2147483647 {
		return ErrInvalidListingPrice
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return ErrListingNotFound
	}
	ct, err := s.pool.Exec(ctx, `UPDATE b2c_listings SET asking_manwon=$2,updated_at=now()
		WHERE id=$1 AND status='active' AND registered_by=$3 AND external_source IS NULL`, n, price, registeredBy)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrListingNotFound
	}
	return nil
}

func (h *Handler) updateListingPrice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Price int `json:"asking_manwon"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	err := h.listings.UpdatePrice(r.Context(), r.PathValue("id"), body.Price, registrant(r.Context()))
	switch {
	case errors.Is(err, ErrListingNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidListingPrice):
		writeErr(w, http.StatusBadRequest, err.Error())
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "price update failed")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": r.PathValue("id"), "asking_manwon": body.Price})
	}
}
